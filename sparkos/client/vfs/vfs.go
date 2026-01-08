package vfs

import (
	"errors"
	"fmt"

	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

// Entry describes a directory entry.
type Entry struct {
	Name string
	Type proto.VFSEntryType
	Size uint32
}

type Client struct {
	vfsCap kernel.Capability

	replyCap     kernel.Capability
	replyCapXfer kernel.Capability
	replyCh      <-chan kernel.Message

	nextRequestID uint32
}

type Writer struct {
	client *Client
	ctx    *kernel.Context

	requestID uint32
}

func New(vfsCap kernel.Capability) *Client {
	return &Client{vfsCap: vfsCap, nextRequestID: 1}
}

func (c *Client) ensureReply(ctx *kernel.Context) error {
	if c.replyCh != nil {
		return nil
	}

	ep := ctx.NewEndpoint(kernel.RightSend | kernel.RightRecv)
	if !ep.Valid() {
		return errors.New("vfs client: failed to allocate reply endpoint")
	}

	ch, ok := ctx.RecvChan(ep.Restrict(kernel.RightRecv))
	if !ok {
		return errors.New("vfs client: failed to receive from reply endpoint")
	}

	c.replyCap = ep
	c.replyCapXfer = ep.Restrict(kernel.RightSend)
	c.replyCh = ch
	return nil
}

func (c *Client) nextID() uint32 {
	id := c.nextRequestID
	c.nextRequestID++
	if c.nextRequestID == 0 {
		c.nextRequestID = 1
	}
	return id
}

func (c *Client) send(ctx *kernel.Context, kind proto.Kind, payload []byte) error {
	for {
		res := ctx.SendToCapResult(c.vfsCap, uint16(kind), payload, c.replyCapXfer)
		switch res {
		case kernel.SendOK:
			return nil
		case kernel.SendErrQueueFull:
			ctx.BlockOnTick()
		default:
			return fmt.Errorf("vfs client send %s: %s", kind, res)
		}
	}
}

func (c *Client) List(ctx *kernel.Context, path string) ([]Entry, error) {
	if err := c.ensureReply(ctx); err != nil {
		return nil, err
	}

	reqID := c.nextID()
	if err := c.send(ctx, proto.MsgVFSList, proto.VFSListPayload(reqID, path)); err != nil {
		return nil, err
	}

	var out []Entry
	for {
		msg := <-c.replyCh
		switch proto.Kind(msg.Kind) {
		case proto.MsgError:
			code, ref, detail, ok := proto.DecodeErrorPayload(msg.Data[:msg.Len])
			if !ok || ref != proto.MsgVFSList {
				return nil, fmt.Errorf("vfs list: %s", code)
			}
			gotID, rest, ok := proto.DecodeErrorDetailWithRequestID(detail)
			if !ok || gotID != reqID {
				return nil, fmt.Errorf("vfs list: %s", code)
			}
			return nil, fmt.Errorf("vfs list: %s: %s", code, string(rest))
		case proto.MsgVFSListResp:
			gotID, done, typ, size, name, ok := proto.DecodeVFSListRespPayload(msg.Data[:msg.Len])
			if !ok || gotID != reqID {
				continue
			}
			if done {
				return out, nil
			}
			out = append(out, Entry{Name: name, Type: typ, Size: size})
		}
	}
}

func (c *Client) Mkdir(ctx *kernel.Context, path string) error {
	if err := c.ensureReply(ctx); err != nil {
		return err
	}

	reqID := c.nextID()
	if err := c.send(ctx, proto.MsgVFSMkdir, proto.VFSMkdirPayload(reqID, path)); err != nil {
		return err
	}

	for {
		msg := <-c.replyCh
		switch proto.Kind(msg.Kind) {
		case proto.MsgError:
			code, ref, detail, ok := proto.DecodeErrorPayload(msg.Data[:msg.Len])
			if !ok || ref != proto.MsgVFSMkdir {
				return fmt.Errorf("vfs mkdir: %s", code)
			}
			gotID, rest, ok := proto.DecodeErrorDetailWithRequestID(detail)
			if !ok || gotID != reqID {
				return fmt.Errorf("vfs mkdir: %s", code)
			}
			return fmt.Errorf("vfs mkdir: %s: %s", code, string(rest))
		case proto.MsgVFSMkdirResp:
			gotID, ok := proto.DecodeVFSMkdirRespPayload(msg.Data[:msg.Len])
			if !ok || gotID != reqID {
				continue
			}
			return nil
		}
	}
}

func (c *Client) Remove(ctx *kernel.Context, path string) error {
	if err := c.ensureReply(ctx); err != nil {
		return err
	}

	reqID := c.nextID()
	if err := c.send(ctx, proto.MsgVFSRemove, proto.VFSRemovePayload(reqID, path)); err != nil {
		return err
	}

	for {
		msg := <-c.replyCh
		switch proto.Kind(msg.Kind) {
		case proto.MsgError:
			code, ref, detail, ok := proto.DecodeErrorPayload(msg.Data[:msg.Len])
			if !ok || ref != proto.MsgVFSRemove {
				return fmt.Errorf("vfs remove: %s", code)
			}
			gotID, rest, ok := proto.DecodeErrorDetailWithRequestID(detail)
			if !ok || gotID != reqID {
				return fmt.Errorf("vfs remove: %s", code)
			}
			return fmt.Errorf("vfs remove: %s: %s", code, string(rest))
		case proto.MsgVFSRemoveResp:
			gotID, ok := proto.DecodeVFSRemoveRespPayload(msg.Data[:msg.Len])
			if !ok || gotID != reqID {
				continue
			}
			return nil
		}
	}
}

func (c *Client) Rename(ctx *kernel.Context, oldPath, newPath string) error {
	if err := c.ensureReply(ctx); err != nil {
		return err
	}

	reqID := c.nextID()
	if err := c.send(ctx, proto.MsgVFSRename, proto.VFSRenamePayload(reqID, oldPath, newPath)); err != nil {
		return err
	}

	for {
		msg := <-c.replyCh
		switch proto.Kind(msg.Kind) {
		case proto.MsgError:
			code, ref, detail, ok := proto.DecodeErrorPayload(msg.Data[:msg.Len])
			if !ok || ref != proto.MsgVFSRename {
				return fmt.Errorf("vfs rename: %s", code)
			}
			gotID, rest, ok := proto.DecodeErrorDetailWithRequestID(detail)
			if !ok || gotID != reqID {
				return fmt.Errorf("vfs rename: %s", code)
			}
			return fmt.Errorf("vfs rename: %s: %s", code, string(rest))
		case proto.MsgVFSRenameResp:
			gotID, ok := proto.DecodeVFSRenameRespPayload(msg.Data[:msg.Len])
			if !ok || gotID != reqID {
				continue
			}
			return nil
		}
	}
}

func (c *Client) Copy(ctx *kernel.Context, srcPath, dstPath string) error {
	if err := c.ensureReply(ctx); err != nil {
		return err
	}

	reqID := c.nextID()
	if err := c.send(ctx, proto.MsgVFSCopy, proto.VFSCopyPayload(reqID, srcPath, dstPath)); err != nil {
		return err
	}

	for {
		msg := <-c.replyCh
		switch proto.Kind(msg.Kind) {
		case proto.MsgError:
			code, ref, detail, ok := proto.DecodeErrorPayload(msg.Data[:msg.Len])
			if !ok || ref != proto.MsgVFSCopy {
				return fmt.Errorf("vfs copy: %s", code)
			}
			gotID, rest, ok := proto.DecodeErrorDetailWithRequestID(detail)
			if !ok || gotID != reqID {
				return fmt.Errorf("vfs copy: %s", code)
			}
			return fmt.Errorf("vfs copy: %s: %s", code, string(rest))
		case proto.MsgVFSCopyResp:
			gotID, done, _, _, ok := proto.DecodeVFSCopyRespPayload(msg.Data[:msg.Len])
			if !ok || gotID != reqID {
				continue
			}
			if done {
				return nil
			}
		}
	}
}

func (c *Client) Stat(ctx *kernel.Context, path string) (proto.VFSEntryType, uint32, error) {
	if err := c.ensureReply(ctx); err != nil {
		return 0, 0, err
	}

	reqID := c.nextID()
	if err := c.send(ctx, proto.MsgVFSStat, proto.VFSStatPayload(reqID, path)); err != nil {
		return 0, 0, err
	}

	for {
		msg := <-c.replyCh
		switch proto.Kind(msg.Kind) {
		case proto.MsgError:
			code, ref, detail, ok := proto.DecodeErrorPayload(msg.Data[:msg.Len])
			if !ok || ref != proto.MsgVFSStat {
				return 0, 0, fmt.Errorf("vfs stat: %s", code)
			}
			gotID, rest, ok := proto.DecodeErrorDetailWithRequestID(detail)
			if !ok || gotID != reqID {
				return 0, 0, fmt.Errorf("vfs stat: %s", code)
			}
			return 0, 0, fmt.Errorf("vfs stat: %s: %s", code, string(rest))
		case proto.MsgVFSStatResp:
			gotID, typ, size, ok := proto.DecodeVFSStatRespPayload(msg.Data[:msg.Len])
			if !ok || gotID != reqID {
				continue
			}
			return typ, size, nil
		}
	}
}

func (c *Client) ReadAt(ctx *kernel.Context, path string, off uint32, maxBytes uint16) ([]byte, bool, error) {
	if err := c.ensureReply(ctx); err != nil {
		return nil, false, err
	}

	reqID := c.nextID()
	if err := c.send(ctx, proto.MsgVFSRead, proto.VFSReadPayload(reqID, path, off, maxBytes)); err != nil {
		return nil, false, err
	}

	for {
		msg := <-c.replyCh
		switch proto.Kind(msg.Kind) {
		case proto.MsgError:
			code, ref, detail, ok := proto.DecodeErrorPayload(msg.Data[:msg.Len])
			if !ok || ref != proto.MsgVFSRead {
				return nil, false, fmt.Errorf("vfs read: %s", code)
			}
			gotID, rest, ok := proto.DecodeErrorDetailWithRequestID(detail)
			if !ok || gotID != reqID {
				return nil, false, fmt.Errorf("vfs read: %s", code)
			}
			return nil, false, fmt.Errorf("vfs read: %s: %s", code, string(rest))
		case proto.MsgVFSReadResp:
			gotID, gotOff, eof, data, ok := proto.DecodeVFSReadRespPayload(msg.Data[:msg.Len])
			if !ok || gotID != reqID || gotOff != off {
				continue
			}
			out := make([]byte, len(data))
			copy(out, data)
			return out, eof, nil
		}
	}
}

func (c *Client) Write(ctx *kernel.Context, path string, mode proto.VFSWriteMode, data []byte) (uint32, error) {
	w, err := c.OpenWriter(ctx, path, mode)
	if err != nil {
		return 0, err
	}
	if _, err := w.Write(data); err != nil {
		_, _ = w.Close()
		return 0, err
	}
	return w.Close()
}

func (c *Client) OpenWriter(ctx *kernel.Context, path string, mode proto.VFSWriteMode) (*Writer, error) {
	if err := c.ensureReply(ctx); err != nil {
		return nil, err
	}

	reqID := c.nextID()
	if err := c.send(ctx, proto.MsgVFSWriteOpen, proto.VFSWriteOpenPayload(reqID, mode, path)); err != nil {
		return nil, err
	}

	for {
		msg := <-c.replyCh
		switch proto.Kind(msg.Kind) {
		case proto.MsgError:
			code, ref, detail, ok := proto.DecodeErrorPayload(msg.Data[:msg.Len])
			if !ok || ref != proto.MsgVFSWriteOpen {
				return nil, fmt.Errorf("vfs write open: %s", code)
			}
			gotID, rest, ok := proto.DecodeErrorDetailWithRequestID(detail)
			if !ok || gotID != reqID {
				return nil, fmt.Errorf("vfs write open: %s", code)
			}
			return nil, fmt.Errorf("vfs write open: %s: %s", code, string(rest))
		case proto.MsgVFSWriteResp:
			gotID, done, _, ok := proto.DecodeVFSWriteRespPayload(msg.Data[:msg.Len])
			if !ok || gotID != reqID || done {
				continue
			}
			return &Writer{client: c, ctx: ctx, requestID: reqID}, nil
		}
	}
}

func (w *Writer) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	maxChunk := kernel.MaxMessageBytes - 6
	written := 0
	for len(p) > 0 {
		chunk := p
		if len(chunk) > maxChunk {
			chunk = chunk[:maxChunk]
		}
		if err := w.client.send(w.ctx, proto.MsgVFSWriteChunk, proto.VFSWriteChunkPayload(w.requestID, chunk)); err != nil {
			return written, err
		}
		written += len(chunk)
		p = p[len(chunk):]
	}
	return written, nil
}

func (w *Writer) Close() (uint32, error) {
	if err := w.client.send(w.ctx, proto.MsgVFSWriteClose, proto.VFSWriteClosePayload(w.requestID)); err != nil {
		return 0, err
	}

	for {
		msg := <-w.client.replyCh
		switch proto.Kind(msg.Kind) {
		case proto.MsgError:
			code, ref, detail, ok := proto.DecodeErrorPayload(msg.Data[:msg.Len])
			if !ok || (ref != proto.MsgVFSWriteChunk && ref != proto.MsgVFSWriteClose) {
				return 0, fmt.Errorf("vfs write: %s", code)
			}
			gotID, rest, ok := proto.DecodeErrorDetailWithRequestID(detail)
			if !ok || gotID != w.requestID {
				return 0, fmt.Errorf("vfs write: %s", code)
			}
			return 0, fmt.Errorf("vfs write: %s: %s", code, string(rest))
		case proto.MsgVFSWriteResp:
			gotID, done, n, ok := proto.DecodeVFSWriteRespPayload(msg.Data[:msg.Len])
			if !ok || gotID != w.requestID || !done {
				continue
			}
			return n, nil
		}
	}
}
