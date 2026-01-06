package shell

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"spark/sparkos/kernel"
)

const maxTextFileBytes = 512 * 1024

func registerTextCommands(r *registry) error {
	for _, cmd := range []command{
		{Name: "head", Usage: "head [-n N] <path>", Desc: "Print the first N lines.", Run: cmdHead},
		{Name: "tail", Usage: "tail [-n N] <path>", Desc: "Print the last N lines.", Run: cmdTail},
		{Name: "wc", Usage: "wc [-lwc] <path...>", Desc: "Count lines/words/bytes.", Run: cmdWc},
		{Name: "grep", Usage: "grep [-in] <pattern> <path>", Desc: "Search lines for a pattern.", Run: cmdGrep},
	} {
		if err := r.register(cmd); err != nil {
			return err
		}
	}
	return nil
}

func cmdHead(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	n := 10
	var pathArg string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-n":
			if i+1 >= len(args) {
				return errors.New("usage: head [-n N] <path>")
			}
			parsed, err := strconv.Atoi(args[i+1])
			if err != nil || parsed < 0 {
				return errors.New("head: invalid -n value")
			}
			n = parsed
			i++
		default:
			if pathArg != "" {
				return errors.New("usage: head [-n N] <path>")
			}
			pathArg = args[i]
		}
	}
	if pathArg == "" {
		return errors.New("usage: head [-n N] <path>")
	}

	abs := s.absPath(pathArg)
	b, err := s.readFileAll(ctx, abs, maxTextFileBytes)
	if err != nil {
		return err
	}

	end := len(b)
	if n == 0 {
		end = 0
	} else {
		lines := 0
		for i, ch := range b {
			if ch == '\n' {
				lines++
				if lines == n {
					end = i + 1
					break
				}
			}
		}
	}
	return s.printString(ctx, string(b[:end]))
}

func cmdTail(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	n := 10
	var pathArg string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-n":
			if i+1 >= len(args) {
				return errors.New("usage: tail [-n N] <path>")
			}
			parsed, err := strconv.Atoi(args[i+1])
			if err != nil || parsed < 0 {
				return errors.New("tail: invalid -n value")
			}
			n = parsed
			i++
		default:
			if pathArg != "" {
				return errors.New("usage: tail [-n N] <path>")
			}
			pathArg = args[i]
		}
	}
	if pathArg == "" {
		return errors.New("usage: tail [-n N] <path>")
	}

	abs := s.absPath(pathArg)
	b, err := s.readFileAll(ctx, abs, maxTextFileBytes)
	if err != nil {
		return err
	}
	if n == 0 {
		return nil
	}

	lines := 0
	start := 0
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] == '\n' {
			lines++
			if lines == n+1 {
				start = i + 1
				break
			}
		}
	}
	return s.printString(ctx, string(b[start:]))
}

func cmdWc(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	showLines := false
	showWords := false
	showBytes := false

	var paths []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") && len(a) > 1 {
			for _, ch := range strings.TrimPrefix(a, "-") {
				switch ch {
				case 'l':
					showLines = true
				case 'w':
					showWords = true
				case 'c':
					showBytes = true
				default:
					return errors.New("usage: wc [-lwc] <path...>")
				}
			}
			continue
		}
		paths = append(paths, a)
	}
	if len(paths) == 0 {
		return errors.New("usage: wc [-lwc] <path...>")
	}
	if !showLines && !showWords && !showBytes {
		showLines, showWords, showBytes = true, true, true
	}

	type counts struct {
		lines uint32
		words uint32
		bytes uint32
	}

	var total counts
	multi := len(paths) > 1
	for _, p := range paths {
		abs := s.absPath(p)
		b, err := s.readFileAll(ctx, abs, maxTextFileBytes)
		if err != nil {
			return err
		}

		var c counts
		c.bytes = uint32(len(b))
		inWord := false
		for _, ch := range b {
			if ch == '\n' {
				c.lines++
			}
			isSpace := ch == ' ' || ch == '\n' || ch == '\t' || ch == '\r' || ch == '\f' || ch == '\v'
			if isSpace {
				inWord = false
				continue
			}
			if !inWord {
				c.words++
				inWord = true
			}
		}

		total.lines += c.lines
		total.words += c.words
		total.bytes += c.bytes

		_ = s.printString(ctx, formatWcLine(showLines, showWords, showBytes, c.lines, c.words, c.bytes, p))
	}
	if multi {
		_ = s.printString(ctx, formatWcLine(showLines, showWords, showBytes, total.lines, total.words, total.bytes, "total"))
	}
	return nil
}

func formatWcLine(showLines, showWords, showBytes bool, lines, words, bytes uint32, label string) string {
	var parts []string
	if showLines {
		parts = append(parts, fmt.Sprintf("%d", lines))
	}
	if showWords {
		parts = append(parts, fmt.Sprintf("%d", words))
	}
	if showBytes {
		parts = append(parts, fmt.Sprintf("%d", bytes))
	}
	parts = append(parts, label)
	return strings.Join(parts, "\t") + "\n"
}

func cmdGrep(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	ignoreCase := false
	withLineNum := false

	var rest []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") && len(a) > 1 {
			for _, ch := range strings.TrimPrefix(a, "-") {
				switch ch {
				case 'i':
					ignoreCase = true
				case 'n':
					withLineNum = true
				default:
					return errors.New("usage: grep [-in] <pattern> <path>")
				}
			}
			continue
		}
		rest = append(rest, a)
	}
	if len(rest) != 2 {
		return errors.New("usage: grep [-in] <pattern> <path>")
	}

	pat := rest[0]
	if ignoreCase {
		pat = strings.ToLower(pat)
	}
	abs := s.absPath(rest[1])
	b, err := s.readFileAll(ctx, abs, maxTextFileBytes)
	if err != nil {
		return err
	}

	lineNo := 1
	start := 0
	for i := 0; i <= len(b); i++ {
		if i == len(b) || b[i] == '\n' {
			line := string(b[start:i])
			hay := line
			if ignoreCase {
				hay = strings.ToLower(hay)
			}
			if strings.Contains(hay, pat) {
				if withLineNum {
					_ = s.printString(ctx, fmt.Sprintf("%d:%s\n", lineNo, line))
				} else {
					_ = s.printString(ctx, line+"\n")
				}
			}
			lineNo++
			start = i + 1
		}
	}
	return nil
}
