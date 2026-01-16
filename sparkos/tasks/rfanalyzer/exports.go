package rfanalyzer

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

const exportDir = "/rf/exports"

func (t *Task) ensureExportDir(ctx *kernel.Context) error {
	if t.vfs == nil {
		return errors.New("vfs unavailable")
	}
	if err := t.vfs.Mkdir(ctx, "/rf"); err != nil {
		if typ, _, statErr := t.vfs.Stat(ctx, "/rf"); statErr != nil || typ != proto.VFSEntryDir {
			return err
		}
	}
	if err := t.vfs.Mkdir(ctx, exportDir); err != nil {
		if typ, _, statErr := t.vfs.Stat(ctx, exportDir); statErr != nil || typ != proto.VFSEntryDir {
			return err
		}
	}
	return nil
}

func (t *Task) exportPath(name, ext string) (string, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", errors.New("empty export name")
	}
	if strings.Contains(name, "/") {
		return "", "", errors.New("export name may not contain '/'")
	}
	safe := sanitizePresetName(name)
	if safe == "" {
		return "", "", errors.New("invalid export name")
	}
	if strings.HasSuffix(safe, ext) {
		safe = strings.TrimSuffix(safe, ext)
	}
	return safe, exportDir + "/" + safe + ext, nil
}

func addrHex(addrLen uint8, addr [5]byte) string {
	if addrLen == 0 {
		return ""
	}
	if addrLen > 5 {
		addrLen = 5
	}
	out := make([]byte, 0, int(addrLen)*2)
	for i := 0; i < int(addrLen); i++ {
		v := addr[i]
		out = append(out, hexDigit(v>>4), hexDigit(v))
	}
	return string(out)
}

func (t *Task) exportReplayCSV(ctx *kernel.Context, name string) error {
	if ctx == nil {
		return errors.New("nil context")
	}
	if t.vfs == nil {
		return errors.New("vfs unavailable")
	}
	if !t.replayActive || t.replay == nil {
		return errors.New("replay not active")
	}
	if err := t.ensureExportDir(ctx); err != nil {
		return fmt.Errorf("ensure export dir: %w", err)
	}
	_, path, err := t.exportPath(name, ".csv")
	if err != nil {
		return err
	}

	header := "t_ms,abs_tick,seq,ch,rate,len,addr,crc_len,crc_ok,delta_ms,flags,payload_hash\n"
	if _, err := t.vfs.Write(ctx, path, proto.VFSWriteTruncate, []byte(header)); err != nil {
		return err
	}

	base := t.replay.startTick
	buf := make([]byte, 0, 16*1024)
	flush := func() error {
		if len(buf) == 0 {
			return nil
		}
		_, err := t.vfs.Write(ctx, path, proto.VFSWriteAppend, buf)
		buf = buf[:0]
		return err
	}

	for _, m := range t.replay.packets {
		rel := uint64(0)
		if m.tick >= base {
			rel = m.tick - base
		}
		line := fmt.Sprintf("%d,%d,%d,%d,%s,%d,%s,%d,%d,%d,%d,%08X\n",
			rel,
			m.tick,
			m.seq,
			m.channel,
			m.rate.String(),
			m.length,
			addrHex(m.addrLen, m.addr),
			m.crcLen,
			bool01(m.crcOK),
			m.deltaMs,
			m.flags,
			m.payloadHash,
		)
		buf = append(buf, line...)
		if len(buf) >= 15*1024 {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

func bool01(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (t *Task) exportReplayRFPKT(ctx *kernel.Context, name string) error {
	if ctx == nil {
		return errors.New("nil context")
	}
	if t.vfs == nil {
		return errors.New("vfs unavailable")
	}
	if !t.replayActive || t.replay == nil {
		return errors.New("replay not active")
	}
	if err := t.ensureExportDir(ctx); err != nil {
		return fmt.Errorf("ensure export dir: %w", err)
	}
	_, path, err := t.exportPath(name, ".rfpkt")
	if err != nil {
		return err
	}

	if _, err := t.vfs.Write(ctx, path, proto.VFSWriteTruncate, []byte("RFPKTv1\n")); err != nil {
		return err
	}

	base := t.replay.startTick
	buf := make([]byte, 0, 16*1024)
	flush := func() error {
		if len(buf) == 0 {
			return nil
		}
		_, err := t.vfs.Write(ctx, path, proto.VFSWriteAppend, buf)
		buf = buf[:0]
		return err
	}

	for i := range t.replay.packets {
		meta := t.replay.packets[i]
		p, err := t.readReplayPacket(ctx, meta)
		if err != nil {
			return err
		}

		rel := uint64(0)
		if meta.tick >= base {
			rel = meta.tick - base
		}
		if rel > 0xFFFFFFFF {
			rel = 0xFFFFFFFF
		}
		raw := rf24RawFrame(&p)
		flags := packetExportFlags(meta)

		var hdr [8]byte
		binary.LittleEndian.PutUint32(hdr[0:4], uint32(rel))
		hdr[4] = meta.channel
		hdr[5] = uint8(meta.rate)
		hdr[6] = flags
		hdr[7] = uint8(len(raw))

		buf = append(buf, hdr[:]...)
		buf = append(buf, raw...)
		if len(buf) >= 15*1024 {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

func (t *Task) exportReplayPCAP(ctx *kernel.Context, name string) error {
	if ctx == nil {
		return errors.New("nil context")
	}
	if t.vfs == nil {
		return errors.New("vfs unavailable")
	}
	if !t.replayActive || t.replay == nil {
		return errors.New("replay not active")
	}
	if err := t.ensureExportDir(ctx); err != nil {
		return fmt.Errorf("ensure export dir: %w", err)
	}
	_, path, err := t.exportPath(name, ".pcap")
	if err != nil {
		return err
	}

	// PCAP global header (little-endian).
	var gh [24]byte
	binary.LittleEndian.PutUint32(gh[0:4], 0xa1b2c3d4)
	binary.LittleEndian.PutUint16(gh[4:6], 2)
	binary.LittleEndian.PutUint16(gh[6:8], 4)
	binary.LittleEndian.PutUint32(gh[16:20], 96)
	binary.LittleEndian.PutUint32(gh[20:24], 147) // DLT_USER0
	if _, err := t.vfs.Write(ctx, path, proto.VFSWriteTruncate, gh[:]); err != nil {
		return err
	}

	base := t.replay.startTick
	buf := make([]byte, 0, 16*1024)
	flush := func() error {
		if len(buf) == 0 {
			return nil
		}
		_, err := t.vfs.Write(ctx, path, proto.VFSWriteAppend, buf)
		buf = buf[:0]
		return err
	}

	for i := range t.replay.packets {
		meta := t.replay.packets[i]
		p, err := t.readReplayPacket(ctx, meta)
		if err != nil {
			return err
		}

		rel := uint64(0)
		if meta.tick >= base {
			rel = meta.tick - base
		}
		tsSec := uint32(rel / 1000)
		tsUsec := uint32((rel % 1000) * 1000)

		raw := rf24RawFrame(&p)
		pktData := make([]byte, 0, 12+len(raw))
		pktData = append(pktData, 'R', 'F', '2', '4')
		pktData = append(pktData,
			1,                // version
			meta.channel,     // channel
			uint8(meta.rate), // rate
			packetExportFlags(meta),
			meta.addrLen,
			meta.length,
			meta.crcLen,
			0,
		)
		pktData = append(pktData, raw...)

		var ph [16]byte
		binary.LittleEndian.PutUint32(ph[0:4], tsSec)
		binary.LittleEndian.PutUint32(ph[4:8], tsUsec)
		binary.LittleEndian.PutUint32(ph[8:12], uint32(len(pktData)))
		binary.LittleEndian.PutUint32(ph[12:16], uint32(len(pktData)))

		buf = append(buf, ph[:]...)
		buf = append(buf, pktData...)
		if len(buf) >= 15*1024 {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

func (t *Task) readReplayPacket(ctx *kernel.Context, meta packetMeta) (packet, error) {
	if ctx == nil {
		return packet{}, errors.New("nil context")
	}
	if t.vfs == nil || t.replay == nil {
		return packet{}, errors.New("vfs/session unavailable")
	}
	hdr, err := readAtExact(ctx, t.vfs, t.replay.path, meta.off, 5)
	if err != nil {
		return packet{}, err
	}
	if sessionRecordType(hdr[0]) != recPacket {
		return packet{}, errors.New("bad packet record")
	}
	recLen := binary.LittleEndian.Uint32(hdr[1:5])
	payload, err := readAtExact(ctx, t.vfs, t.replay.path, meta.off+5, int(recLen))
	if err != nil {
		return packet{}, err
	}
	p, ok := decodeSessionPacket(payload)
	if !ok {
		return packet{}, errors.New("bad packet payload")
	}
	return p, nil
}

func packetExportFlags(m packetMeta) byte {
	f := byte(0)
	if m.crcLen > 0 {
		f |= 1 << 1
	}
	if m.crcOK {
		f |= 1 << 0
	}
	if (m.flags & pktFlagRetry) != 0 {
		f |= 1 << 2
	}
	if (m.flags & pktFlagBurst) != 0 {
		f |= 1 << 3
	}
	return f
}

func rf24RawFrame(p *packet) []byte {
	if p == nil {
		return nil
	}
	ln := 1 + int(p.addrLen) + int(p.length) + int(p.crcLen)
	if ln < 1 {
		ln = 1
	}
	raw := make([]byte, 0, ln)
	raw = append(raw, 0x55)
	for i := 0; i < int(p.addrLen) && i < len(p.addr); i++ {
		raw = append(raw, p.addr[i])
	}
	for i := 0; i < int(p.length) && i < len(p.payload); i++ {
		raw = append(raw, p.payload[i])
	}
	for i := 0; i < int(p.crcLen) && i < len(p.crc); i++ {
		raw = append(raw, p.crc[i])
	}
	return raw
}
