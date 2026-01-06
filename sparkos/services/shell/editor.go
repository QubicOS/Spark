package shell

import "spark/sparkos/kernel"

func (s *Service) moveLeft(ctx *kernel.Context) {
	if s.cursor <= 0 {
		return
	}
	_ = s.writeString(ctx, "\x1b[D")
	s.cursor--
}

func (s *Service) moveRight(ctx *kernel.Context) {
	if s.cursor >= len(s.line) {
		return
	}
	_ = s.writeString(ctx, "\x1b[C")
	s.cursor++
}

func (s *Service) home(ctx *kernel.Context) {
	for s.cursor > 0 {
		s.moveLeft(ctx)
	}
}

func (s *Service) end(ctx *kernel.Context) {
	for s.cursor < len(s.line) {
		s.moveRight(ctx)
	}
}

func (s *Service) insertRune(ctx *kernel.Context, r rune) {
	if s.cursor == len(s.line) {
		s.line = append(s.line, r)
		s.cursor++
		_ = s.writeString(ctx, string(r))
		return
	}
	s.line = append(s.line, 0)
	copy(s.line[s.cursor+1:], s.line[s.cursor:])
	s.line[s.cursor] = r
	_ = s.writeString(ctx, string(r))
	s.cursor++
	_ = s.redrawFromCursor(ctx)
}

func (s *Service) deleteForward(ctx *kernel.Context) {
	if s.cursor >= len(s.line) {
		return
	}
	s.line = append(s.line[:s.cursor], s.line[s.cursor+1:]...)
	_ = s.redrawFromCursor(ctx)
}

func (s *Service) redrawFromCursor(ctx *kernel.Context) error {
	tail := s.line[s.cursor:]
	if err := s.writeString(ctx, string(tail)); err != nil {
		return err
	}
	if err := s.writeString(ctx, "\x1b[K"); err != nil {
		return err
	}
	for range tail {
		if err := s.writeString(ctx, "\x1b[D"); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) backspace(ctx *kernel.Context) {
	if len(s.line) == 0 || s.cursor == 0 {
		return
	}
	s.cursor--
	s.line = append(s.line[:s.cursor], s.line[s.cursor+1:]...)

	_ = s.writeString(ctx, "\x1b[D")
	_ = s.redrawFromCursor(ctx)
}

func (s *Service) killLeft(ctx *kernel.Context) {
	if s.cursor <= 0 {
		return
	}
	s.line = append([]rune{}, s.line[s.cursor:]...)
	s.cursor = 0
	_ = s.redrawLine(ctx)
}

func (s *Service) deletePrevWord(ctx *kernel.Context) {
	if s.cursor <= 0 {
		return
	}
	i := s.cursor
	for i > 0 && s.line[i-1] == ' ' {
		i--
	}
	for i > 0 && s.line[i-1] != ' ' {
		i--
	}
	if i == s.cursor {
		return
	}
	s.line = append(s.line[:i], s.line[s.cursor:]...)
	s.cursor = i
	_ = s.redrawLine(ctx)
}

func (s *Service) cancelLine(ctx *kernel.Context) {
	if len(s.line) == 0 {
		_ = s.writeString(ctx, "\n")
		_ = s.prompt(ctx)
		return
	}
	_ = s.writeString(ctx, "\n")
	s.line = s.line[:0]
	s.cursor = 0
	s.utf8buf = s.utf8buf[:0]
	_ = s.prompt(ctx)
}
