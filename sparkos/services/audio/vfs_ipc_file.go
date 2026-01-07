package audio

import (
	"errors"
	"fmt"
	"io"

	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

type ipcFile struct {
	ctx kernel.Context

	vfsCap      kernel.Capability
	replyCap    kernel.Capability
	replyCapOut kernel.Capability
	replyCh     <-chan kernel.Message

	path string
	off  uint32

	nextRequestID uint32
	payloadBuf    [kernel.MaxMessageBytes]byte
}

func newIPCFile(ctx *kernel.Context, vfsCap kernel.Capability, path string) (*ipcFile, error) {
	if !vfsCap.Valid() {
		return nil, errors.New("audio: vfs capability is missing")
	}
	if len(path) > kernel.MaxMessageBytes-12 {
		return nil, errors.New("audio: path too long")
	}

	ep := ctx.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	if !ep.Valid() {
		return nil, errors.New("audio: allocate vfs reply endpoint")
	}
	ch, ok := ctx.RecvChan(ep.Restrict(kernel.RightRecv))
	if !ok {
		return nil, errors.New("audio: recv vfs reply endpoint")
	}

	return &ipcFile{
		ctx:           *ctx,
		vfsCap:        vfsCap,
		replyCap:      ep,
		replyCapOut:   ep.Restrict(kernel.RightSend),
		replyCh:       ch,
		path:          path,
		nextRequestID: 1,
	}, nil
}

func (f *ipcFile) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	maxRead := len(p)
	maxPayload := kernel.MaxMessageBytes - 11
	if maxRead > maxPayload {
		maxRead = maxPayload
	}

	reqID := f.nextRequestID
	f.nextRequestID++
	if f.nextRequestID == 0 {
		f.nextRequestID = 1
	}

	payload, ok := proto.VFSReadPayloadInto(f.payloadBuf[:], reqID, f.path, f.off, uint16(maxRead))
	if !ok {
		return 0, errors.New("audio: vfs read payload too large")
	}

	for {
		res := f.ctx.SendToCapResult(f.vfsCap, uint16(proto.MsgVFSRead), payload, f.replyCapOut)
		switch res {
		case kernel.SendOK:
			goto sent
		case kernel.SendErrQueueFull:
			f.ctx.BlockOnTick()
		default:
			return 0, fmt.Errorf("audio: vfs read send: %s", res)
		}
	}
sent:

	for {
		msg := <-f.replyCh
		switch proto.Kind(msg.Kind) {
		case proto.MsgError:
			code, ref, detail, ok := proto.DecodeErrorPayload(msg.Data[:msg.Len])
			if !ok || ref != proto.MsgVFSRead {
				return 0, fmt.Errorf("audio: vfs read: %s", code)
			}
			gotID, rest, ok := proto.DecodeErrorDetailWithRequestID(detail)
			if !ok || gotID != reqID {
				continue
			}
			return 0, fmt.Errorf("audio: vfs read: %s: %s", code, string(rest))
		case proto.MsgVFSReadResp:
			gotID, gotOff, eof, data, ok := proto.DecodeVFSReadRespPayload(msg.Data[:msg.Len])
			if !ok || gotID != reqID || gotOff != f.off {
				continue
			}
			n := copy(p, data)
			f.off += uint32(n)
			if eof && n == 0 {
				return 0, io.EOF
			}
			return n, nil
		}
	}
}

func (f *ipcFile) Seek(offset int64, whence int) (int64, error) {
	var next int64
	switch whence {
	case io.SeekStart:
		next = offset
	case io.SeekCurrent:
		next = int64(f.off) + offset
	default:
		return 0, errors.New("audio: seek whence not supported")
	}
	if next < 0 {
		return 0, errors.New("audio: negative seek")
	}
	if next > int64(^uint32(0)) {
		return 0, errors.New("audio: seek overflow")
	}
	f.off = uint32(next)
	return next, nil
}
