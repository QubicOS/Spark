package rfanalyzer

import (
	"encoding/binary"
	"errors"
	"strings"

	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

type annotation struct {
	startTick uint64
	endTick   uint64
	tag       string
	note      string
}

func (a annotation) durationMs() uint64 {
	if a.endTick <= a.startTick {
		return 0
	}
	return a.endTick - a.startTick
}

func (t *Task) selectedPacketTick() (uint64, bool) {
	if t.replayActive {
		meta, ok := t.filteredReplayPacketMetaByIndex(t.snifferSel)
		if ok && meta.tick != 0 {
			return meta.tick, true
		}
		return 0, false
	}
	p, ok := t.filteredLivePacketByIndex(t.snifferSel)
	if !ok || p == nil || p.tick == 0 {
		return 0, false
	}
	return p.tick, true
}

func (t *Task) beginAnnotationAt(startTick uint64) {
	t.annotPending = annotation{
		startTick: startTick,
		endTick:   startTick,
		tag:       t.annotLastTag,
	}
	if strings.TrimSpace(t.annotPending.tag) == "" {
		t.annotPending.tag = "note"
	}
	t.openPrompt(promptAnnotTag, "Annotation tag", t.annotPending.tag)
}

func (t *Task) addAnnotation(ctx *kernel.Context, a annotation) error {
	a.tag = strings.TrimSpace(a.tag)
	a.note = strings.TrimSpace(a.note)
	if a.tag == "" {
		a.tag = "note"
	}
	if len(a.tag) > 16 {
		a.tag = a.tag[:16]
	}
	if len(a.note) > 32 {
		a.note = a.note[:32]
	}
	if a.endTick < a.startTick {
		a.endTick = a.startTick
	}

	t.annotLastTag = a.tag

	if t.replayActive && t.replay != nil {
		if err := t.appendAnnotationToSession(ctx, t.replay, a); err != nil {
			t.replayErr = err.Error()
			return err
		}
		t.replay.annotations = append(t.replay.annotations, a)
		t.replay.noteTick(a.startTick)
		t.replay.noteTick(a.endTick)
		t.invalidate(dirtyAnalysis | dirtyStatus)
		return nil
	}

	t.pushLiveAnnotation(a)
	if t.recording {
		t.recordAnnotation(a)
	}
	t.invalidate(dirtyAnalysis | dirtyStatus)
	return nil
}

func (t *Task) pushLiveAnnotation(a annotation) {
	if len(t.annotations) == 0 {
		return
	}
	t.annotations[t.annotHead] = a
	t.annotHead++
	if t.annotHead >= len(t.annotations) {
		t.annotHead = 0
	}
	if t.annotCount < len(t.annotations) {
		t.annotCount++
	}
}

func (t *Task) appendAnnotationToSession(ctx *kernel.Context, s *session, a annotation) error {
	if ctx == nil {
		return errors.New("nil context")
	}
	if t.vfs == nil {
		return errors.New("vfs unavailable")
	}
	if s == nil || s.path == "" {
		return errors.New("session unavailable")
	}

	tag := []byte(a.tag)
	if len(tag) > 32 {
		tag = tag[:32]
	}
	note := []byte(a.note)
	if len(note) > 64 {
		note = note[:64]
	}

	payloadLen := 8 + 8 + 1 + len(tag) + 1 + len(note)
	buf := make([]byte, 0, 5+payloadLen)
	buf = append(buf, byte(recAnnotation), 0, 0, 0, 0)
	var tmp [8]byte
	binary.LittleEndian.PutUint64(tmp[:], a.startTick)
	buf = append(buf, tmp[:]...)
	binary.LittleEndian.PutUint64(tmp[:], a.endTick)
	buf = append(buf, tmp[:]...)
	buf = append(buf, byte(len(tag)))
	buf = append(buf, tag...)
	buf = append(buf, byte(len(note)))
	buf = append(buf, note...)
	binary.LittleEndian.PutUint32(buf[1:5], uint32(payloadLen))

	if _, err := t.vfs.Write(ctx, s.path, proto.VFSWriteAppend, buf); err != nil {
		return err
	}
	s.size += uint32(len(buf))
	return nil
}

func (t *Task) visibleAnnotations(now uint64, limit int) []annotation {
	if limit <= 0 {
		return nil
	}
	if t.replayActive && t.replay != nil {
		return visibleAnnotationsFromSlice(t.replay.annotations, now, limit)
	}
	return visibleAnnotationsFromRing(t.annotations, t.annotHead, t.annotCount, limit)
}

func visibleAnnotationsFromSlice(src []annotation, now uint64, limit int) []annotation {
	if len(src) == 0 {
		return nil
	}
	last := -1
	for i := range src {
		if src[i].startTick != 0 && src[i].startTick <= now {
			last = i
		}
	}
	if last < 0 {
		last = len(src) - 1
		if last < 0 {
			return nil
		}
	}
	start := last - (limit - 1)
	if start < 0 {
		start = 0
	}
	out := make([]annotation, 0, limit)
	for i := last; i >= start; i-- {
		out = append(out, src[i])
	}
	return out
}

func visibleAnnotationsFromRing(src [64]annotation, head, count, limit int) []annotation {
	if count <= 0 {
		return nil
	}
	if limit > count {
		limit = count
	}
	out := make([]annotation, 0, limit)
	idx := head - 1
	if idx < 0 {
		idx = len(src) - 1
	}
	for i := 0; i < limit; i++ {
		a := src[idx]
		out = append(out, a)
		idx--
		if idx < 0 {
			idx = len(src) - 1
		}
	}
	return out
}
