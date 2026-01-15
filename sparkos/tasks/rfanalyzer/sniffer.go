package rfanalyzer

import (
	"fmt"
)

const maxPackets = 256

type packet struct {
	seq  uint32
	tick uint64

	channel uint8
	rate    rfDataRate

	addrLen uint8
	addr    [5]byte

	length  uint8
	payload [32]byte

	crcLen uint8
	crc    [2]byte
	crcOK  bool
}

func rateShort(r rfDataRate) byte {
	switch r {
	case rfRate2M:
		return '2'
	case rfRate1M:
		return '1'
	case rfRate250K:
		return '0'
	default:
		return '?'
	}
}

func (p *packet) addrSuffix3() string {
	if p.addrLen == 0 {
		return "------"
	}
	start := int(p.addrLen) - 3
	if start < 0 {
		start = 0
	}
	for start+3 > int(p.addrLen) {
		start--
		if start < 0 {
			start = 0
			break
		}
	}
	b0 := byte(0)
	b1 := byte(0)
	b2 := byte(0)
	if start < int(p.addrLen) {
		b0 = p.addr[start]
	}
	if start+1 < int(p.addrLen) {
		b1 = p.addr[start+1]
	}
	if start+2 < int(p.addrLen) {
		b2 = p.addr[start+2]
	}
	return fmt.Sprintf("%02X%02X%02X", b0, b1, b2)
}

func (p *packet) crcText() string {
	if p.crcLen == 0 {
		return "--"
	}
	if p.crcOK {
		return "OK"
	}
	return "!!"
}

func (t *Task) appendPacket(p packet) {
	if t.pktSeq == 0 {
		t.pktSeq = 1
	}
	p.seq = t.pktSeq
	t.pktSeq++

	if t.pktCount < maxPackets {
		t.packets[t.pktHead] = p
		t.pktHead++
		if t.pktHead >= maxPackets {
			t.pktHead = 0
		}
		t.pktCount++
	} else {
		t.packets[t.pktHead] = p
		t.pktHead++
		if t.pktHead >= maxPackets {
			t.pktHead = 0
		}
		t.pktDropped++
	}

	t.pktSecCount++
	t.invalidate(dirtySniffer | dirtyProtocol | dirtyStatus)
	t.reconcileSnifferSelection()
}

func (t *Task) tickPacketsPerSecond(now uint64) {
	if t.pktSecStart == 0 {
		t.pktSecStart = now
		return
	}
	if now-t.pktSecStart < 1000 {
		return
	}
	t.pktsPerSec = t.pktSecCount
	t.pktSecCount = 0
	t.pktSecStart = now
	t.invalidate(dirtyStatus)
}

func (t *Task) packetByDisplayIndex(i int) (*packet, bool) {
	if i < 0 || i >= t.pktCount {
		return nil, false
	}
	idx := t.pktHead - 1 - i
	for idx < 0 {
		idx += maxPackets
	}
	if idx >= maxPackets {
		idx %= maxPackets
	}
	return &t.packets[idx], true
}

func (t *Task) packetPassesFilters(p *packet) bool {
	if p == nil {
		return false
	}

	switch t.filterCRC {
	case filterCRCOK:
		if !p.crcOK {
			return false
		}
	case filterCRCBad:
		if p.crcOK {
			return false
		}
	}

	switch t.filterChannel {
	case filterChannelSelected:
		if int(p.channel) != t.selectedChannel {
			return false
		}
	case filterChannelRange:
		if int(p.channel) < t.channelRangeLo || int(p.channel) > t.channelRangeHi {
			return false
		}
	}

	if t.filterMinLen > 0 && int(p.length) < t.filterMinLen {
		return false
	}
	if t.filterMaxLen > 0 && int(p.length) > t.filterMaxLen {
		return false
	}

	if t.filterAddrLen > 0 {
		if int(p.addrLen) < t.filterAddrLen {
			return false
		}
		for i := 0; i < t.filterAddrLen; i++ {
			if p.addr[i] != t.filterAddr[i] {
				return false
			}
		}
	}

	return true
}

func (t *Task) filteredCount() int {
	n := 0
	for i := 0; i < t.pktCount; i++ {
		p, ok := t.packetByDisplayIndex(i)
		if !ok {
			continue
		}
		if t.packetPassesFilters(p) {
			n++
		}
	}
	return n
}

func (t *Task) filteredPacketByIndex(idx int) (*packet, bool) {
	if idx < 0 {
		return nil, false
	}
	seen := 0
	for i := 0; i < t.pktCount; i++ {
		p, ok := t.packetByDisplayIndex(i)
		if !ok {
			continue
		}
		if !t.packetPassesFilters(p) {
			continue
		}
		if seen == idx {
			return p, true
		}
		seen++
	}
	return nil, false
}

func (t *Task) reconcileSnifferSelection() {
	if t.snifferSelSeq == 0 {
		if p, ok := t.filteredPacketByIndex(0); ok {
			t.snifferSel = 0
			t.snifferSelSeq = p.seq
		}
		return
	}
	seen := 0
	for i := 0; i < t.pktCount; i++ {
		p, ok := t.packetByDisplayIndex(i)
		if !ok || !t.packetPassesFilters(p) {
			continue
		}
		if p.seq == t.snifferSelSeq {
			t.snifferSel = seen
			return
		}
		seen++
	}
	if p, ok := t.filteredPacketByIndex(0); ok {
		t.snifferSel = 0
		t.snifferSelSeq = p.seq
	} else {
		t.snifferSel = 0
		t.snifferSelSeq = 0
	}
}

func (t *Task) maybeCapturePacket(ch int, energy uint8, tick uint64) {
	if t.capturePaused {
		return
	}
	if energy < 180 {
		return
	}

	// Avoid flooding: energy-gated probabilistic capture.
	if (t.rng & 0xFF) > uint32(energy) {
		return
	}

	// Bias towards "nRF24-like" narrow carriers.
	if ch != 76 && ch != 91 && ch != t.selectedChannel {
		if (t.rng & 0x03) != 0 {
			return
		}
	}

	p := packet{
		tick:    tick,
		channel: uint8(ch),
		rate:    t.dataRate,
		addrLen: 5,
	}

	switch {
	case ch == 76:
		p.addr = [5]byte{0xE7, 0xE7, 0xE7, 0xE7, 0xE7}
	case ch == 91:
		p.addr = [5]byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE}
	default:
		p.addr = [5]byte{0x11, 0x22, 0x33, 0x44, 0x55}
	}

	ln := int((t.rng>>8)%25) + 8
	if ln > 32 {
		ln = 32
	}
	p.length = uint8(ln)

	x := t.rng
	for i := 0; i < ln; i++ {
		// xorshift32
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		p.payload[i] = byte(x)
	}
	t.rng = x

	switch t.crcMode {
	case rfCRC1B:
		p.crcLen = 1
	case rfCRC2B:
		p.crcLen = 2
	default:
		p.crcLen = 0
	}
	if p.crcLen > 0 {
		p.crc[0] = byte(x >> 8)
		p.crc[1] = byte(x >> 16)
		// Mostly OK, occasionally bad.
		p.crcOK = (x & 0x0F) != 0
	}

	t.appendPacket(p)
}

func (t *Task) snifferListRows() int {
	l := t.computeLayout()
	inner := l.sniffer.inset(2, 2)
	rows := int((inner.h - 2*t.fontHeight) / t.fontHeight)
	if rows < 1 {
		return 1
	}
	return rows
}

func (t *Task) moveSnifferSelection(delta int) {
	total := t.filteredCount()
	if total <= 0 {
		return
	}
	sel := t.snifferSel + delta
	if sel < 0 {
		sel = 0
	}
	if sel >= total {
		sel = total - 1
	}
	if sel == t.snifferSel {
		return
	}
	t.snifferSel = sel
	if p, ok := t.filteredPacketByIndex(t.snifferSel); ok && p != nil {
		t.snifferSelSeq = p.seq
	}

	rows := t.snifferListRows()
	maxTop := total - rows
	if maxTop < 0 {
		maxTop = 0
	}
	if t.snifferTop < 0 {
		t.snifferTop = 0
	}
	if t.snifferTop > maxTop {
		t.snifferTop = maxTop
	}
	if t.snifferSel < t.snifferTop {
		t.snifferTop = t.snifferSel
	}
	if t.snifferSel >= t.snifferTop+rows {
		t.snifferTop = t.snifferSel - rows + 1
		if t.snifferTop > maxTop {
			t.snifferTop = maxTop
		}
	}

	t.invalidate(dirtySniffer | dirtyProtocol | dirtyStatus)
}
