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

type sweepIndex struct {
	off  uint32
	tick uint64
}

type packetMeta struct {
	off  uint32
	tick uint64
	seq  uint32

	channel uint8
	rate    rfDataRate

	addrLen uint8
	addr    [5]byte

	length uint8

	payloadHash uint32

	crcLen uint8
	crcOK  bool
}

type sessionConfigEvent struct {
	tick            uint64
	selectedChannel int
	cfg             cfgSnapshot
}

type session struct {
	name string
	path string
	size uint32

	startTick uint64
	endTick   uint64

	sweeps  []sweepIndex
	packets []packetMeta
	configs []sessionConfigEvent
}

func (t *Task) loadSession(ctx *kernel.Context, input string) (*session, error) {
	if ctx == nil {
		return nil, errors.New("nil context")
	}
	if t.vfs == nil {
		return nil, errors.New("vfs unavailable")
	}
	name, path, err := t.resolveSessionInput(input)
	if err != nil {
		return nil, err
	}

	typ, size, err := t.vfs.Stat(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if typ != proto.VFSEntryFile {
		return nil, fmt.Errorf("not a file: %s", path)
	}
	if size < uint32(len(sessionMagic)) {
		return nil, fmt.Errorf("too small: %d bytes", size)
	}

	magic, err := readAtExact(ctx, t.vfs, path, 0, len(sessionMagic))
	if err != nil {
		return nil, err
	}
	if string(magic) != sessionMagic {
		return nil, fmt.Errorf("bad session header (expected %q)", sessionMagic)
	}

	s := &session{
		name:      name,
		path:      path,
		size:      size,
		startTick: ^uint64(0),
		endTick:   0,
	}

	off := uint32(len(sessionMagic))
	for off < size {
		hdr, err := readAtExact(ctx, t.vfs, path, off, 5)
		if err != nil {
			return nil, err
		}
		recType := sessionRecordType(hdr[0])
		recLen := binary.LittleEndian.Uint32(hdr[1:5])
		off += 5
		if recLen == 0 {
			continue
		}
		if recLen > 64*1024 {
			return nil, fmt.Errorf("record too large: %d bytes", recLen)
		}
		if off+recLen > size {
			return nil, fmt.Errorf("truncated record at %d", off)
		}
		payload, err := readAtExact(ctx, t.vfs, path, off, int(recLen))
		if err != nil {
			return nil, err
		}

		switch recType {
		case recConfig:
			ev, ok := decodeSessionConfig(payload)
			if ok {
				s.configs = append(s.configs, ev)
				s.noteTick(ev.tick)
			}
		case recSweep:
			tick, ok := decodeSessionSweepTick(payload)
			if ok {
				s.sweeps = append(s.sweeps, sweepIndex{off: off - 5, tick: tick})
				s.noteTick(tick)
			}
		case recPacket:
			meta, ok := decodeSessionPacketMeta(payload)
			if ok {
				meta.off = off - 5
				s.packets = append(s.packets, meta)
				s.noteTick(meta.tick)
			}
		default:
			// ignore unknown record types
		}

		off += recLen
	}

	if s.startTick == ^uint64(0) {
		s.startTick = 0
	}
	if len(s.sweeps) == 0 && len(s.packets) == 0 {
		return nil, errors.New("empty session")
	}
	return s, nil
}

func (t *Task) resolveSessionInput(input string) (string, string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", errors.New("empty session name")
	}
	if strings.Contains(input, "/") {
		name := input
		if i := strings.LastIndexByte(input, '/'); i >= 0 && i+1 < len(input) {
			name = input[i+1:]
		}
		return name, input, nil
	}
	safe, path, err := t.sessionPath(input)
	if err != nil {
		return "", "", err
	}
	return safe, path, nil
}

func (s *session) noteTick(tick uint64) {
	if tick == 0 {
		return
	}
	if tick < s.startTick {
		s.startTick = tick
	}
	if tick > s.endTick {
		s.endTick = tick
	}
}

func readAtExact(ctx *kernel.Context, vfs *vfsclient.Client, path string, off uint32, n int) ([]byte, error) {
	if vfs == nil {
		return nil, errors.New("vfs unavailable")
	}
	if n <= 0 {
		return nil, nil
	}
	out := make([]byte, 0, n)
	cur := off
	for len(out) < n {
		limit := uint16(n - len(out))
		if limit > uint16(kernel.MaxMessageBytes) {
			limit = uint16(kernel.MaxMessageBytes)
		}
		chunk, eof, err := vfs.ReadAt(ctx, path, cur, limit)
		if err != nil {
			return nil, fmt.Errorf("read %s at %d: %w", path, cur, err)
		}
		if len(chunk) == 0 {
			if eof {
				return nil, fmt.Errorf("unexpected EOF reading %s at %d", path, cur)
			}
			return nil, fmt.Errorf("short read %s at %d", path, cur)
		}
		out = append(out, chunk...)
		cur += uint32(len(chunk))
	}
	if len(out) > n {
		out = out[:n]
	}
	return out, nil
}

func decodeSessionSweepTick(payload []byte) (uint64, bool) {
	if len(payload) < 8+numChannels {
		return 0, false
	}
	return binary.LittleEndian.Uint64(payload[0:8]), true
}

func decodeSessionConfig(payload []byte) (sessionConfigEvent, bool) {
	if len(payload) < 19 {
		return sessionConfigEvent{}, false
	}
	ev := sessionConfigEvent{}
	ev.tick = binary.LittleEndian.Uint64(payload[0:8])
	ev.cfg.channelRangeLo = int(payload[8])
	ev.cfg.channelRangeHi = int(payload[9])
	ev.selectedChannel = int(payload[10])
	ev.cfg.dwellTimeMs = int(binary.LittleEndian.Uint16(payload[11:13]))
	ev.cfg.scanStep = int(payload[13])
	ev.cfg.dataRate = rfDataRate(payload[14])
	ev.cfg.crcMode = rfCRCMode(payload[15])
	ev.cfg.autoAck = payload[16] != 0
	ev.cfg.powerLevel = rfPowerLevel(payload[17])
	ev.cfg.wfPalette = wfPalette(payload[18])
	return ev, true
}

func decodeSessionPacketMeta(payload []byte) (packetMeta, bool) {
	// tick:u64 seq:u32 ch:u8 rate:u8 addrLen:u8 addr addrLen length:u8 payload length crcLen:u8 crc crcLen crcOK:u8
	var m packetMeta
	if len(payload) < 8+4+1+1+1+1+1 {
		return packetMeta{}, false
	}
	off := 0
	m.tick = binary.LittleEndian.Uint64(payload[off : off+8])
	off += 8
	m.seq = binary.LittleEndian.Uint32(payload[off : off+4])
	off += 4
	m.channel = payload[off]
	off++
	m.rate = rfDataRate(payload[off])
	off++
	m.addrLen = payload[off]
	off++
	if m.addrLen > 5 {
		m.addrLen = 5
	}
	if off+int(m.addrLen) > len(payload) {
		return packetMeta{}, false
	}
	copy(m.addr[:], payload[off:off+int(m.addrLen)])
	off += int(m.addrLen)
	if off >= len(payload) {
		return packetMeta{}, false
	}
	m.length = payload[off]
	off++
	if m.length > 32 {
		m.length = 32
	}
	if off+int(m.length) > len(payload) {
		return packetMeta{}, false
	}
	m.payloadHash = fnv1a32(payload[off : off+int(m.length)])
	off += int(m.length)
	if off >= len(payload) {
		return packetMeta{}, false
	}
	m.crcLen = payload[off]
	off++
	if m.crcLen > 2 {
		m.crcLen = 2
	}
	if off+int(m.crcLen) > len(payload) {
		return packetMeta{}, false
	}
	off += int(m.crcLen)
	if off >= len(payload) {
		return packetMeta{}, false
	}
	m.crcOK = payload[off] != 0
	return m, true
}

func decodeSessionPacket(payload []byte) (packet, bool) {
	var p packet
	if len(payload) < 8+4+1+1+1+1+1 {
		return packet{}, false
	}
	off := 0
	p.tick = binary.LittleEndian.Uint64(payload[off : off+8])
	off += 8
	p.seq = binary.LittleEndian.Uint32(payload[off : off+4])
	off += 4
	p.channel = payload[off]
	off++
	p.rate = rfDataRate(payload[off])
	off++
	p.addrLen = payload[off]
	off++
	if p.addrLen > 5 {
		p.addrLen = 5
	}
	if off+int(p.addrLen) > len(payload) {
		return packet{}, false
	}
	copy(p.addr[:], payload[off:off+int(p.addrLen)])
	off += int(p.addrLen)
	if off >= len(payload) {
		return packet{}, false
	}
	p.length = payload[off]
	off++
	if p.length > 32 {
		p.length = 32
	}
	if off+int(p.length) > len(payload) {
		return packet{}, false
	}
	copy(p.payload[:], payload[off:off+int(p.length)])
	off += int(p.length)
	if off >= len(payload) {
		return packet{}, false
	}
	p.crcLen = payload[off]
	off++
	if p.crcLen > 2 {
		p.crcLen = 2
	}
	if off+int(p.crcLen) > len(payload) {
		return packet{}, false
	}
	copy(p.crc[:], payload[off:off+int(p.crcLen)])
	off += int(p.crcLen)
	if off >= len(payload) {
		return packet{}, false
	}
	p.crcOK = payload[off] != 0
	return p, true
}
