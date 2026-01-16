package rfanalyzer

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	vfsclient "spark/sparkos/client/vfs"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

const (
	sessionMagic             = "RFLOGv1\n"
	sessionDir               = "/rf/sessions"
	sessionExt               = ".rflog"
	recordFlushIntervalTicks = 250
	maxRecordBuf             = 32 * 1024
)

type sessionRecordType uint8

const (
	recConfig sessionRecordType = 1 + iota
	recSweep
	recPacket
	recAnnotation
	recEvent
)

func (t *Task) ensureSessionDir(ctx *kernel.Context) error {
	if t.vfs == nil {
		return errors.New("vfs unavailable")
	}
	if err := t.vfs.Mkdir(ctx, "/rf"); err != nil {
		if typ, _, statErr := t.vfs.Stat(ctx, "/rf"); statErr != nil || typ != proto.VFSEntryDir {
			return err
		}
	}
	if err := t.vfs.Mkdir(ctx, sessionDir); err != nil {
		if typ, _, statErr := t.vfs.Stat(ctx, sessionDir); statErr != nil || typ != proto.VFSEntryDir {
			return err
		}
	}
	return nil
}

func (t *Task) sessionPath(name string) (string, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", errors.New("empty session name")
	}
	if strings.Contains(name, "/") {
		return "", "", errors.New("session name may not contain '/'")
	}
	safe := sanitizePresetName(name)
	if safe == "" {
		return "", "", errors.New("invalid session name")
	}
	if strings.HasSuffix(safe, sessionExt) {
		safe = strings.TrimSuffix(safe, sessionExt)
	}
	return safe, sessionDir + "/" + safe + sessionExt, nil
}

func (t *Task) startRecording(ctx *kernel.Context, name string) error {
	if t.vfs == nil {
		return errors.New("vfs unavailable")
	}
	if t.recording {
		return errors.New("recording already active")
	}
	if err := t.ensureSessionDir(ctx); err != nil {
		return fmt.Errorf("ensure session dir: %w", err)
	}
	safe, path, err := t.sessionPath(name)
	if err != nil {
		return err
	}

	header := []byte(sessionMagic)
	if _, err := t.vfs.Write(ctx, path, proto.VFSWriteTruncate, header); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	t.recording = true
	t.recordName = safe
	t.recordPath = path
	t.recordBuf = t.recordBuf[:0]
	t.recordNextFlushTick = ctx.NowTick() + recordFlushIntervalTicks
	t.recordSweeps = 0
	t.recordPackets = 0
	t.recordBytes = uint32(len(header))
	t.recordErr = ""

	t.recordConfig(ctx.NowTick())
	t.invalidate(dirtyStatus | dirtyRFControl)
	return nil
}

func (t *Task) stopRecording(ctx *kernel.Context) error {
	if !t.recording {
		return nil
	}
	t.flushRecording(ctx, ctx.NowTick(), true)
	t.recording = false
	t.recordName = ""
	t.recordPath = ""
	t.recordBuf = t.recordBuf[:0]
	t.recordNextFlushTick = 0
	t.invalidate(dirtyStatus | dirtyRFControl)
	return nil
}

func (t *Task) flushRecording(ctx *kernel.Context, now uint64, force bool) {
	if !t.recording || t.vfs == nil || t.recordPath == "" {
		return
	}
	if !force {
		if now < t.recordNextFlushTick && len(t.recordBuf) < maxRecordBuf {
			return
		}
	}
	if len(t.recordBuf) == 0 {
		t.recordNextFlushTick = now + recordFlushIntervalTicks
		return
	}
	_, err := t.vfs.Write(ctx, t.recordPath, proto.VFSWriteAppend, t.recordBuf)
	if err != nil {
		t.recordErr = err.Error()
		t.recording = false
		t.recordBuf = t.recordBuf[:0]
		t.invalidate(dirtyStatus | dirtyRFControl)
		return
	}
	t.recordBytes += uint32(len(t.recordBuf))
	t.recordBuf = t.recordBuf[:0]
	t.recordNextFlushTick = now + recordFlushIntervalTicks
}

func (t *Task) recordStart(kind sessionRecordType) int {
	if !t.recording {
		return -1
	}
	start := len(t.recordBuf)
	t.recordBuf = append(t.recordBuf, byte(kind), 0, 0, 0, 0)
	return start
}

func (t *Task) recordFinish(start int) {
	if start < 0 || start+5 > len(t.recordBuf) {
		return
	}
	payloadLen := len(t.recordBuf) - (start + 5)
	binary.LittleEndian.PutUint32(t.recordBuf[start+1:start+5], uint32(payloadLen))
}

func (t *Task) recordU8(v uint8) {
	t.recordBuf = append(t.recordBuf, v)
}

func (t *Task) recordU16(v uint16) {
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], v)
	t.recordBuf = append(t.recordBuf, b[:]...)
}

func (t *Task) recordU32(v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	t.recordBuf = append(t.recordBuf, b[:]...)
}

func (t *Task) recordU64(v uint64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], v)
	t.recordBuf = append(t.recordBuf, b[:]...)
}

func (t *Task) recordConfig(now uint64) {
	start := t.recordStart(recConfig)
	if start < 0 {
		return
	}

	t.recordU64(now)
	t.recordU8(uint8(t.channelRangeLo))
	t.recordU8(uint8(t.channelRangeHi))
	t.recordU8(uint8(t.selectedChannel))
	t.recordU16(uint16(t.dwellTimeMs))
	t.recordU8(uint8(clampInt(t.scanSpeedScalar, 1, 10)))
	t.recordU8(uint8(t.dataRate))
	t.recordU8(uint8(t.crcMode))
	if t.autoAck {
		t.recordU8(1)
	} else {
		t.recordU8(0)
	}
	t.recordU8(uint8(t.powerLevel))
	t.recordU8(uint8(t.wfPalette))

	t.recordFinish(start)
}

func (t *Task) recordSweep(now uint64) {
	start := t.recordStart(recSweep)
	if start < 0 {
		return
	}

	t.recordU64(now)
	for i := 0; i < numChannels; i++ {
		t.recordU8(t.energyAvg[i])
	}
	t.recordFinish(start)
	t.recordSweeps++
}

func (t *Task) recordPacket(p packet) {
	start := t.recordStart(recPacket)
	if start < 0 {
		return
	}

	t.recordU64(p.tick)
	t.recordU32(p.seq)
	t.recordU8(p.channel)
	t.recordU8(uint8(p.rate))
	t.recordU8(p.addrLen)
	for i := 0; i < int(p.addrLen) && i < len(p.addr); i++ {
		t.recordU8(p.addr[i])
	}
	t.recordU8(p.length)
	for i := 0; i < int(p.length) && i < len(p.payload); i++ {
		t.recordU8(p.payload[i])
	}
	t.recordU8(p.crcLen)
	for i := 0; i < int(p.crcLen) && i < len(p.crc); i++ {
		t.recordU8(p.crc[i])
	}
	if p.crcOK {
		t.recordU8(1)
	} else {
		t.recordU8(0)
	}

	t.recordFinish(start)
	t.recordPackets++
}

func (t *Task) recordAnnotation(a annotation) {
	start := t.recordStart(recAnnotation)
	if start < 0 {
		return
	}

	t.recordU64(a.startTick)
	t.recordU64(a.endTick)

	tag := []byte(a.tag)
	if len(tag) > 32 {
		tag = tag[:32]
	}
	note := []byte(a.note)
	if len(note) > 64 {
		note = note[:64]
	}

	t.recordU8(uint8(len(tag)))
	t.recordBuf = append(t.recordBuf, tag...)
	t.recordU8(uint8(len(note)))
	t.recordBuf = append(t.recordBuf, note...)

	t.recordFinish(start)
}

func (t *Task) vfsClient() *vfsclient.Client {
	return t.vfs
}
