package basic

import (
	"fmt"
	"sort"
	"strings"
)

type progLine struct {
	no   int
	text string
}

func (l progLine) String() string {
	if l.text == "" {
		return fmt.Sprintf("%d", l.no)
	}
	return fmt.Sprintf("%d %s", l.no, l.text)
}

type program struct {
	lines []progLine
}

func (p *program) upsertLine(no int, text string) {
	text = strings.TrimSpace(text)
	i, ok := p.find(no)
	if ok {
		p.lines[i].text = text
		return
	}
	p.lines = append(p.lines, progLine{no: no, text: text})
	sort.Slice(p.lines, func(i, j int) bool { return p.lines[i].no < p.lines[j].no })
}

func (p *program) deleteLine(no int) {
	i, ok := p.find(no)
	if !ok {
		return
	}
	copy(p.lines[i:], p.lines[i+1:])
	p.lines = p.lines[:len(p.lines)-1]
}

func (p *program) find(no int) (idx int, ok bool) {
	i := sort.Search(len(p.lines), func(i int) bool { return p.lines[i].no >= no })
	if i >= len(p.lines) || p.lines[i].no != no {
		return 0, false
	}
	return i, true
}
