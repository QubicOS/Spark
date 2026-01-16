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

	deltaMs uint16
	flags   uint8

	channel uint8
	rate    rfDataRate

	addrLen uint8
	addr    [5]byte

	length uint8

	payloadHash   uint32
	payloadPrefix [payloadPrefixBytes]byte

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

	sweeps      []sweepIndex
	packets     []packetMeta
	configs     []sessionConfigEvent
	annotations []annotation
	bucketMs    uint32
	bandOccPct  []uint8

	// Aggregated stats (full-session).
	sweepCount uint32
	occCount   [numChannels]uint32
	energySum  [numChannels]uint64
	pktCount   [numChannels]uint32
	pktBad     [numChannels]uint32
}

type sessionDevTrack struct {
	used bool

	addrLen uint8
	addr    [5]byte

	lastTick uint64

	lastPayloadHash uint32
	lastPayloadTick uint64
}

func deriveSessionPacketMeta(devs *[maxDevices]sessionDevTrack, m packetMeta) (uint16, uint8) {
	if devs == nil || m.addrLen == 0 {
		return 0, 0
	}
	addrLen := m.addrLen
	if addrLen > 5 {
		addrLen = 5
	}

	matchIdx := -1
	for i := range devs {
		if !devs[i].used || devs[i].addrLen != addrLen {
			continue
		}
		match := true
		for j := 0; j < int(addrLen); j++ {
			if devs[i].addr[j] != m.addr[j] {
				match = false
				break
			}
		}
		if match {
			matchIdx = i
			break
		}
	}

	if matchIdx < 0 {
		evict := -1
		var oldest uint64
		for i := range devs {
			if !devs[i].used {
				evict = i
				break
			}
			if evict == -1 || devs[i].lastTick < oldest {
				oldest = devs[i].lastTick
				evict = i
			}
		}
		if evict < 0 {
			return 0, 0
		}
		d := &devs[evict]
		*d = sessionDevTrack{
			used:            true,
			addrLen:         addrLen,
			addr:            m.addr,
			lastTick:        m.tick,
			lastPayloadHash: m.payloadHash,
			lastPayloadTick: m.tick,
		}
		return 0, 0
	}

	d := &devs[matchIdx]
	deltaMs := uint16(0)
	flags := uint8(0)
	if d.lastTick != 0 && m.tick > d.lastTick {
		dt := m.tick - d.lastTick
		if dt > 0xFFFF {
			dt = 0xFFFF
		}
		deltaMs = uint16(dt)
		if dt <= 12 {
			flags |= pktFlagBurst
		}
	}
	if d.lastPayloadHash != 0 &&
		d.lastPayloadHash == m.payloadHash &&
		m.tick > d.lastPayloadTick &&
		(m.tick-d.lastPayloadTick) <= anaRetryWindowTicks {
		flags |= pktFlagRetry
	}
	d.lastTick = m.tick
	d.lastPayloadHash = m.payloadHash
	d.lastPayloadTick = m.tick

	return deltaMs, flags
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

	var devTrack [maxDevices]sessionDevTrack
	const bucketMs = uint64(60_000)
	var bucketOccSum []uint32
	var bucketSweepCount []uint32

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
				s.sweepCount++
				occNow := 0
				for ch := 0; ch < numChannels; ch++ {
					v := payload[8+ch]
					s.energySum[ch] += uint64(v)
					if v >= anaOccThreshold {
						s.occCount[ch]++
						occNow++
					}
				}
				if s.startTick != 0 && bucketMs > 0 && tick >= s.startTick {
					b := int((tick - s.startTick) / bucketMs)
					if b < 0 {
						b = 0
					}
					for len(bucketOccSum) <= b {
						bucketOccSum = append(bucketOccSum, 0)
						bucketSweepCount = append(bucketSweepCount, 0)
					}
					bucketOccSum[b] += uint32(occNow)
					bucketSweepCount[b]++
				}
			}
		case recPacket:
			meta, ok := decodeSessionPacketMeta(payload)
			if ok {
				meta.off = off - 5
				meta.deltaMs, meta.flags = deriveSessionPacketMeta(&devTrack, meta)
				s.packets = append(s.packets, meta)
				s.noteTick(meta.tick)
				ch := int(meta.channel)
				if ch >= 0 && ch < numChannels {
					s.pktCount[ch]++
					if meta.crcLen > 0 && !meta.crcOK {
						s.pktBad[ch]++
					}
				}
			}
		case recAnnotation:
			a, ok := decodeSessionAnnotation(payload)
			if ok {
				s.annotations = append(s.annotations, a)
				s.noteTick(a.startTick)
				s.noteTick(a.endTick)
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

	if len(bucketSweepCount) > 0 {
		s.bucketMs = uint32(bucketMs)
		s.bandOccPct = make([]uint8, len(bucketSweepCount))
		for i := range bucketSweepCount {
			cnt := bucketSweepCount[i]
			if cnt == 0 {
				continue
			}
			denom := uint64(cnt) * uint64(numChannels)
			if denom == 0 {
				continue
			}
			pct := uint64(bucketOccSum[i]) * 100 / denom
			if pct > 100 {
				pct = 100
			}
			s.bandOccPct[i] = uint8(pct)
		}
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

func decodeSessionAnnotation(payload []byte) (annotation, bool) {
	if len(payload) < 8+8+1+1 {
		return annotation{}, false
	}
	off := 0
	a := annotation{}
	a.startTick = binary.LittleEndian.Uint64(payload[off : off+8])
	off += 8
	a.endTick = binary.LittleEndian.Uint64(payload[off : off+8])
	off += 8
	if off >= len(payload) {
		return annotation{}, false
	}
	tagLen := int(payload[off])
	off++
	if tagLen < 0 || off+tagLen > len(payload) {
		return annotation{}, false
	}
	a.tag = string(payload[off : off+tagLen])
	off += tagLen
	if off >= len(payload) {
		return annotation{}, false
	}
	noteLen := int(payload[off])
	off++
	if noteLen < 0 || off+noteLen > len(payload) {
		return annotation{}, false
	}
	a.note = string(payload[off : off+noteLen])
	return a, true
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
	pl := payload[off : off+int(m.length)]
	m.payloadHash = fnv1a32(pl)
	copy(m.payloadPrefix[:], pl)
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
	p.payloadHash = fnv1a32(payload[off : off+int(p.length)])
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
