//go:build !tinygo

package vfs

import (
	"errors"
	"fmt"

	"spark/hal"
	"spark/sparkos/fs/littlefs"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

type Service struct {
	inCap kernel.Capability
	flash hal.Flash

	fs *littlefs.FS

	writers map[uint32]*writeSession
}

type writeSession struct {
	reply  kernel.Capability
	writer *littlefs.Writer
}

func New(flash hal.Flash, inCap kernel.Capability) *Service {
	return &Service{flash: flash, inCap: inCap}
}

func (s *Service) Run(ctx *kernel.Context) {
	ch, ok := ctx.RecvChan(s.inCap)
	if !ok {
		return
	}

	if s.flash != nil {
		fs, err := littlefs.New(s.flash, littlefs.Options{})
		if err == nil {
			err = fs.MountOrFormat()
		}
		if err == nil {
			s.fs = fs
		}
	}

	if s.writers == nil {
		s.writers = make(map[uint32]*writeSession)
	}

	for msg := range ch {
		switch proto.Kind(msg.Kind) {
		case proto.MsgVFSList:
			s.handleList(ctx, msg)
		case proto.MsgVFSMkdir:
			s.handleMkdir(ctx, msg)
		case proto.MsgVFSRemove:
			s.handleRemove(ctx, msg)
		case proto.MsgVFSRename:
			s.handleRename(ctx, msg)
		case proto.MsgVFSCopy:
			s.handleCopy(ctx, msg)
		case proto.MsgVFSStat:
			s.handleStat(ctx, msg)
		case proto.MsgVFSRead:
			s.handleRead(ctx, msg)
		case proto.MsgVFSWriteOpen:
			s.handleWriteOpen(ctx, msg)
		case proto.MsgVFSWriteChunk:
			s.handleWriteChunk(ctx, msg)
		case proto.MsgVFSWriteClose:
			s.handleWriteClose(ctx, msg)
		}
	}
}

func (s *Service) handleList(ctx *kernel.Context, msg kernel.Message) {
	reply := msg.Cap
	requestID, path, ok := proto.DecodeVFSListPayload(msg.Data[:msg.Len])
	if !ok {
		_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSList, 0, "decode list")
		return
	}

	fs := s.fs
	if fs == nil {
		_ = s.sendErr(ctx, reply, proto.ErrInternal, proto.MsgVFSList, requestID, "vfs not ready")
		return
	}

	if err := fs.ListDir(path, func(name string, info littlefs.Info) bool {
		typ := proto.VFSEntryUnknown
		switch info.Type {
		case littlefs.TypeFile:
			typ = proto.VFSEntryFile
		case littlefs.TypeDir:
			typ = proto.VFSEntryDir
		}
		_ = s.send(ctx, reply, proto.MsgVFSListResp, proto.VFSListRespPayload(requestID, false, typ, info.Size, name))
		return true
	}); err != nil {
		_ = s.sendErr(ctx, reply, mapVFSError(err), proto.MsgVFSList, requestID, err.Error())
		return
	}

	_ = s.send(ctx, reply, proto.MsgVFSListResp, proto.VFSListRespPayload(requestID, true, proto.VFSEntryUnknown, 0, ""))
}

func (s *Service) handleMkdir(ctx *kernel.Context, msg kernel.Message) {
	reply := msg.Cap
	requestID, path, ok := proto.DecodeVFSMkdirPayload(msg.Data[:msg.Len])
	if !ok {
		_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSMkdir, 0, "decode mkdir")
		return
	}

	fs := s.fs
	if fs == nil {
		_ = s.sendErr(ctx, reply, proto.ErrInternal, proto.MsgVFSMkdir, requestID, "vfs not ready")
		return
	}

	if err := fs.Mkdir(path); err != nil {
		_ = s.sendErr(ctx, reply, mapVFSError(err), proto.MsgVFSMkdir, requestID, err.Error())
		return
	}
	_ = s.send(ctx, reply, proto.MsgVFSMkdirResp, proto.VFSMkdirRespPayload(requestID))
}

func (s *Service) handleRemove(ctx *kernel.Context, msg kernel.Message) {
	reply := msg.Cap
	requestID, path, ok := proto.DecodeVFSRemovePayload(msg.Data[:msg.Len])
	if !ok {
		_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSRemove, 0, "decode remove")
		return
	}

	fs := s.fs
	if fs == nil {
		_ = s.sendErr(ctx, reply, proto.ErrInternal, proto.MsgVFSRemove, requestID, "vfs not ready")
		return
	}

	if err := fs.Remove(path); err != nil {
		_ = s.sendErr(ctx, reply, mapVFSError(err), proto.MsgVFSRemove, requestID, err.Error())
		return
	}
	_ = s.send(ctx, reply, proto.MsgVFSRemoveResp, proto.VFSRemoveRespPayload(requestID))
}

func (s *Service) handleRename(ctx *kernel.Context, msg kernel.Message) {
	reply := msg.Cap
	requestID, oldPath, newPath, ok := proto.DecodeVFSRenamePayload(msg.Data[:msg.Len])
	if !ok {
		_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSRename, 0, "decode rename")
		return
	}

	fs := s.fs
	if fs == nil {
		_ = s.sendErr(ctx, reply, proto.ErrInternal, proto.MsgVFSRename, requestID, "vfs not ready")
		return
	}

	if err := fs.Rename(oldPath, newPath); err != nil {
		_ = s.sendErr(ctx, reply, mapVFSError(err), proto.MsgVFSRename, requestID, err.Error())
		return
	}
	_ = s.send(ctx, reply, proto.MsgVFSRenameResp, proto.VFSRenameRespPayload(requestID))
}

func (s *Service) handleCopy(ctx *kernel.Context, msg kernel.Message) {
	reply := msg.Cap
	requestID, srcPath, dstPath, ok := proto.DecodeVFSCopyPayload(msg.Data[:msg.Len])
	if !ok {
		_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSCopy, 0, "decode copy")
		return
	}
	if !reply.Valid() {
		return
	}

	fs := s.fs
	if fs == nil {
		_ = s.sendErr(ctx, reply, proto.ErrInternal, proto.MsgVFSCopy, requestID, "vfs not ready")
		return
	}

	info, err := fs.Stat(srcPath)
	if err != nil {
		_ = s.sendErr(ctx, reply, mapVFSError(err), proto.MsgVFSCopy, requestID, err.Error())
		return
	}
	if info.Type != littlefs.TypeFile {
		_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSCopy, requestID, "source is not a file")
		return
	}

	w, err := fs.OpenWriter(dstPath, littlefs.WriteTruncate)
	if err != nil {
		_ = s.sendErr(ctx, reply, mapVFSError(err), proto.MsgVFSCopy, requestID, err.Error())
		return
	}
	defer func() { _ = w.Close() }()

	const bufSize = 4096
	buf := make([]byte, bufSize)

	var copied uint32
	total := info.Size
	lastPct := -1

	sendProgress := func(done bool) {
		pct := 100
		if total > 0 {
			pct = int((uint64(copied) * 100) / uint64(total))
			if pct < 0 {
				pct = 0
			}
			if pct > 100 {
				pct = 100
			}
		}
		if !done && pct == lastPct {
			return
		}
		lastPct = pct
		_ = s.send(ctx, reply, proto.MsgVFSCopyResp, proto.VFSCopyRespPayload(requestID, done, copied, total))
	}

	sendProgress(false)

	var off uint32
	for {
		n, eof, err := fs.ReadAt(srcPath, buf, off)
		if err != nil {
			_ = s.sendErr(ctx, reply, mapVFSError(err), proto.MsgVFSCopy, requestID, err.Error())
			return
		}
		if n > 0 {
			written, err := w.Write(buf[:n])
			if err != nil {
				_ = s.sendErr(ctx, reply, mapVFSError(err), proto.MsgVFSCopy, requestID, err.Error())
				return
			}
			if written != n {
				_ = s.sendErr(ctx, reply, proto.ErrInternal, proto.MsgVFSCopy, requestID, "short write")
				return
			}
			off += uint32(n)
			copied += uint32(n)
			sendProgress(false)
		}
		if eof {
			break
		}
	}

	if err := w.Close(); err != nil {
		_ = s.sendErr(ctx, reply, mapVFSError(err), proto.MsgVFSCopy, requestID, err.Error())
		return
	}
	sendProgress(true)
}

func (s *Service) handleStat(ctx *kernel.Context, msg kernel.Message) {
	reply := msg.Cap
	requestID, path, ok := proto.DecodeVFSStatPayload(msg.Data[:msg.Len])
	if !ok {
		_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSStat, 0, "decode stat")
		return
	}

	fs := s.fs
	if fs == nil {
		_ = s.sendErr(ctx, reply, proto.ErrInternal, proto.MsgVFSStat, requestID, "vfs not ready")
		return
	}

	info, err := fs.Stat(path)
	if err != nil {
		_ = s.sendErr(ctx, reply, mapVFSError(err), proto.MsgVFSStat, requestID, err.Error())
		return
	}

	typ := proto.VFSEntryUnknown
	switch info.Type {
	case littlefs.TypeFile:
		typ = proto.VFSEntryFile
	case littlefs.TypeDir:
		typ = proto.VFSEntryDir
	}

	_ = s.send(ctx, reply, proto.MsgVFSStatResp, proto.VFSStatRespPayload(requestID, typ, info.Size))
}

func (s *Service) handleRead(ctx *kernel.Context, msg kernel.Message) {
	reply := msg.Cap
	requestID, path, off, maxBytes, ok := proto.DecodeVFSReadPayload(msg.Data[:msg.Len])
	if !ok {
		_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSRead, 0, "decode read")
		return
	}

	fs := s.fs
	if fs == nil {
		_ = s.sendErr(ctx, reply, proto.ErrInternal, proto.MsgVFSRead, requestID, "vfs not ready")
		return
	}

	max := int(maxBytes)
	maxPayload := kernel.MaxMessageBytes - 11
	if max > maxPayload {
		max = maxPayload
	}
	if max < 0 {
		max = 0
	}
	buf := make([]byte, max)

	n, eof, err := fs.ReadAt(path, buf, off)
	if err != nil {
		_ = s.sendErr(ctx, reply, mapVFSError(err), proto.MsgVFSRead, requestID, err.Error())
		return
	}

	_ = s.send(ctx, reply, proto.MsgVFSReadResp, proto.VFSReadRespPayload(requestID, off, eof, buf[:n]))
}

func (s *Service) handleWriteOpen(ctx *kernel.Context, msg kernel.Message) {
	reply := msg.Cap
	requestID, mode, path, ok := proto.DecodeVFSWriteOpenPayload(msg.Data[:msg.Len])
	if !ok {
		_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSWriteOpen, 0, "decode write open")
		return
	}
	if !reply.Valid() {
		return
	}

	fs := s.fs
	if fs == nil {
		_ = s.sendErr(ctx, reply, proto.ErrInternal, proto.MsgVFSWriteOpen, requestID, "vfs not ready")
		return
	}

	if prev := s.writers[requestID]; prev != nil {
		_ = prev.writer.Close()
		delete(s.writers, requestID)
	}

	wmode := littlefs.WriteTruncate
	if mode == proto.VFSWriteAppend {
		wmode = littlefs.WriteAppend
	}

	w, err := fs.OpenWriter(path, wmode)
	if err != nil {
		_ = s.sendErr(ctx, reply, mapVFSError(err), proto.MsgVFSWriteOpen, requestID, err.Error())
		return
	}

	s.writers[requestID] = &writeSession{reply: reply, writer: w}
	_ = s.send(ctx, reply, proto.MsgVFSWriteResp, proto.VFSWriteRespPayload(requestID, false, 0))
}

func (s *Service) handleWriteChunk(ctx *kernel.Context, msg kernel.Message) {
	requestID, data, ok := proto.DecodeVFSWriteChunkPayload(msg.Data[:msg.Len])
	if !ok {
		return
	}

	sess := s.writers[requestID]
	if sess == nil || sess.writer == nil {
		return
	}

	n, err := sess.writer.Write(data)
	if err != nil {
		_ = s.sendErr(ctx, sess.reply, mapVFSError(err), proto.MsgVFSWriteChunk, requestID, err.Error())
		_ = sess.writer.Close()
		delete(s.writers, requestID)
		return
	}
	if n != len(data) {
		_ = s.sendErr(ctx, sess.reply, proto.ErrInternal, proto.MsgVFSWriteChunk, requestID, "short write")
		_ = sess.writer.Close()
		delete(s.writers, requestID)
		return
	}
}

func (s *Service) handleWriteClose(ctx *kernel.Context, msg kernel.Message) {
	requestID, ok := proto.DecodeVFSWriteClosePayload(msg.Data[:msg.Len])
	if !ok {
		return
	}

	sess := s.writers[requestID]
	if sess == nil || sess.writer == nil {
		return
	}
	delete(s.writers, requestID)

	if err := sess.writer.Close(); err != nil {
		_ = s.sendErr(ctx, sess.reply, mapVFSError(err), proto.MsgVFSWriteClose, requestID, err.Error())
		return
	}
	_ = s.send(ctx, sess.reply, proto.MsgVFSWriteResp, proto.VFSWriteRespPayload(requestID, true, sess.writer.BytesWritten()))
}

func (s *Service) send(ctx *kernel.Context, to kernel.Capability, kind proto.Kind, payload []byte) error {
	for {
		res := ctx.SendToCapResult(to, uint16(kind), payload, kernel.Capability{})
		switch res {
		case kernel.SendOK:
			return nil
		case kernel.SendErrQueueFull:
			ctx.BlockOnTick()
		default:
			return fmt.Errorf("vfs send %s: %s", kind, res)
		}
	}
}

func (s *Service) sendErr(
	ctx *kernel.Context,
	to kernel.Capability,
	code proto.ErrCode,
	ref proto.Kind,
	requestID uint32,
	detail string,
) error {
	if !to.Valid() {
		return nil
	}
	d := proto.ErrorDetailWithRequestID(requestID, []byte(detail))
	return s.send(ctx, to, proto.MsgError, proto.ErrorPayload(code, ref, d))
}

func mapVFSError(err error) proto.ErrCode {
	switch {
	case errors.Is(err, littlefs.ErrNotFound):
		return proto.ErrNotFound
	case errors.Is(err, littlefs.ErrExists):
		return proto.ErrBusy
	case errors.Is(err, littlefs.ErrNotEmpty):
		return proto.ErrBusy
	case errors.Is(err, littlefs.ErrNoSpace):
		return proto.ErrOverflow
	case errors.Is(err, littlefs.ErrNotDir), errors.Is(err, littlefs.ErrIsDir), errors.Is(err, littlefs.ErrInvalid):
		return proto.ErrBadMessage
	default:
		return proto.ErrInternal
	}
}
