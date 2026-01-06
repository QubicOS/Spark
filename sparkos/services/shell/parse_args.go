package shell

type redirection struct {
	Path   string
	Append bool
}

func parseArgs(line string) (args []string, redir redirection, ok bool) {
	type state uint8
	const (
		stNone state = iota
		stSingle
		stDouble
		stEscape
	)

	var cur []rune
	st := stNone

	flush := func() {
		if len(cur) == 0 {
			return
		}
		args = append(args, string(cur))
		cur = cur[:0]
	}

	emitOp := func(op string) {
		flush()
		args = append(args, op)
	}

	runes := []rune(line)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch st {
		case stEscape:
			cur = append(cur, r)
			st = stNone
			continue
		case stSingle:
			if r == '\'' {
				st = stNone
				continue
			}
			cur = append(cur, r)
			continue
		case stDouble:
			if r == '"' {
				st = stNone
				continue
			}
			if r == '\\' {
				st = stEscape
				continue
			}
			cur = append(cur, r)
			continue
		}

		switch r {
		case '\\':
			st = stEscape
		case '\'':
			st = stSingle
		case '"':
			st = stDouble
		case ' ', '\t':
			flush()
		case '>':
			if i+1 < len(runes) && runes[i+1] == '>' {
				i++
				emitOp(">>")
			} else {
				emitOp(">")
			}
		default:
			cur = append(cur, r)
		}
	}
	if st != stNone {
		return nil, redirection{}, false
	}
	flush()

	// Handle simple trailing redirection: ... > file OR ... >> file.
	if len(args) >= 3 {
		op := args[len(args)-2]
		if op == ">" || op == ">>" {
			redir.Path = args[len(args)-1]
			redir.Append = op == ">>"
			args = args[:len(args)-2]
		}
	}
	return args, redir, true
}
