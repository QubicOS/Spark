package shell

import (
	"errors"
	"fmt"
	"strings"

	logclient "spark/sparkos/client/logger"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

func (s *Service) writeString(ctx *kernel.Context, str string) error {
	return s.writeBytes(ctx, []byte(str))
}

func (s *Service) printString(ctx *kernel.Context, str string) error {
	if err := s.writeString(ctx, str); err != nil {
		return err
	}
	s.addScrollback(str)
	return nil
}

func (s *Service) addScrollback(str string) {
	lines := strings.Split(str, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return
	}
	s.scrollback = append(s.scrollback, lines...)
	if len(s.scrollback) <= scrollbackMaxLines {
		return
	}
	excess := len(s.scrollback) - scrollbackMaxLines
	copy(s.scrollback, s.scrollback[excess:])
	s.scrollback = s.scrollback[:scrollbackMaxLines]
}

func (s *Service) writeBytes(ctx *kernel.Context, b []byte) error {
	for len(b) > 0 {
		chunk := b
		if len(chunk) > kernel.MaxMessageBytes {
			chunk = chunk[:kernel.MaxMessageBytes]
		}
		if err := s.sendToTerm(ctx, proto.MsgTermWrite, chunk); err != nil {
			return err
		}
		b = b[len(chunk):]
	}
	return nil
}

func (s *Service) sendToTerm(ctx *kernel.Context, kind proto.Kind, payload []byte) error {
	res := ctx.SendToCapRetry(s.termCap, uint16(kind), payload, kernel.Capability{}, 500)
	switch res {
	case kernel.SendOK:
		return nil
	case kernel.SendErrQueueFull:
		if s.logCap.Valid() {
			_ = logclient.Log(ctx, s.logCap, fmt.Sprintf("shell: term send %s: queue full", kind))
		}
		return fmt.Errorf("shell term send %s: queue full", kind)
	default:
		if s.logCap.Valid() {
			_ = logclient.Log(ctx, s.logCap, fmt.Sprintf("shell: term send %s: %s", kind, res))
		}
		return fmt.Errorf("shell term send: %s", res)
	}
}

func (s *Service) sendToMux(ctx *kernel.Context, kind proto.Kind, payload []byte) error {
	if !s.muxCap.Valid() {
		return errors.New("no consolemux capability")
	}
	res := ctx.SendToCapRetry(s.muxCap, uint16(kind), payload, kernel.Capability{}, 500)
	switch res {
	case kernel.SendOK:
		return nil
	case kernel.SendErrQueueFull:
		if s.logCap.Valid() {
			_ = logclient.Log(ctx, s.logCap, fmt.Sprintf("shell: consolemux send %s: queue full", kind))
		}
		return fmt.Errorf("shell consolemux send %s: queue full", kind)
	default:
		if s.logCap.Valid() {
			_ = logclient.Log(ctx, s.logCap, fmt.Sprintf("shell: consolemux send %s: %s", kind, res))
		}
		return fmt.Errorf("shell consolemux send: %s", res)
	}
}
