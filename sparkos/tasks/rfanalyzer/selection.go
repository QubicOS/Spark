package rfanalyzer

func (t *Task) selectedPacketAddr() (uint8, [5]byte, bool) {
	if t.replayActive {
		meta, ok := t.filteredReplayPacketMetaByIndex(t.snifferSel)
		if !ok || meta.addrLen == 0 {
			return 0, [5]byte{}, false
		}
		return meta.addrLen, meta.addr, true
	}
	p, ok := t.filteredLivePacketByIndex(t.snifferSel)
	if !ok || p == nil || p.addrLen == 0 {
		return 0, [5]byte{}, false
	}
	return p.addrLen, p.addr, true
}
