package vfs

import (
	"errors"
	"fmt"
	"strings"

	"spark/hal"
	"spark/sparkos/fs/littlefs"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

type Service struct {
	inCap kernel.Capability
	flash hal.Flash

	fs *littlefs.FS
	sd fsHandle

	writers map[uint32]*writeSession
}

type writeSession struct {
	reply  kernel.Capability
	writer writeHandle
}

func New(flash hal.Flash, inCap kernel.Capability) *Service {
	return &Service{flash: flash, inCap: inCap}
}

type writeHandle interface {
	Write(p []byte) (n int, err error)
	Close() error
	BytesWritten() uint32
}

type fsHandle interface {
	ListDir(path string, fn func(name string, info littlefs.Info) bool) error
	Mkdir(path string) error
	Remove(path string) error
	Rename(oldPath, newPath string) error
	Stat(path string) (littlefs.Info, error)
	ReadAt(path string, p []byte, off uint32) (n int, eof bool, err error)
	OpenWriter(path string, mode littlefs.WriteMode) (writeHandle, error)
}

type flashFS struct {
	fs *littlefs.FS
}

func (f flashFS) ListDir(path string, fn func(name string, info littlefs.Info) bool) error {
	return f.fs.ListDir(path, fn)
}
func (f flashFS) Mkdir(path string) error                 { return f.fs.Mkdir(path) }
func (f flashFS) Remove(path string) error                { return f.fs.Remove(path) }
func (f flashFS) Rename(oldPath, newPath string) error    { return f.fs.Rename(oldPath, newPath) }
func (f flashFS) Stat(path string) (littlefs.Info, error) { return f.fs.Stat(path) }
func (f flashFS) ReadAt(path string, p []byte, off uint32) (int, bool, error) {
	return f.fs.ReadAt(path, p, off)
}
func (f flashFS) OpenWriter(path string, mode littlefs.WriteMode) (writeHandle, error) {
	return f.fs.OpenWriter(path, mode)
}

func (s *Service) Run(ctx *kernel.Context) {
	ch, ok := ctx.RecvChan(s.inCap)
	if !ok {
		return
	}

	s.sd = s.initSD(ctx)
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
	requestID, path, ok := proto.DecodeVFSListPayload(msg.Payload())
	if !ok {
		_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSList, 0, "decode list")
		return
	}

	if path == "/" && s.fs == nil {
		if s.sd != nil {
			_ = s.send(ctx, reply, proto.MsgVFSListResp, proto.VFSListRespPayload(requestID, false, proto.VFSEntryDir, 0, "sd"))
			_ = s.send(ctx, reply, proto.MsgVFSListResp, proto.VFSListRespPayload(requestID, true, proto.VFSEntryUnknown, 0, ""))
			return
		}
		_ = s.sendErr(ctx, reply, proto.ErrInternal, proto.MsgVFSList, requestID, "vfs not ready")
		return
	}

	backend, rel, ok := s.resolve(path)
	if !ok {
		if isSDPath(path) {
			_ = s.sendErr(ctx, reply, proto.ErrNotFound, proto.MsgVFSList, requestID, "sd not available")
		} else if s.fs == nil {
			_ = s.sendErr(ctx, reply, proto.ErrInternal, proto.MsgVFSList, requestID, "vfs not ready")
		} else {
			_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSList, requestID, "invalid path")
		}
		return
	}

	if path == "/" && s.sd != nil {
		_ = s.send(ctx, reply, proto.MsgVFSListResp, proto.VFSListRespPayload(requestID, false, proto.VFSEntryDir, 0, "sd"))
	}

	if err := backend.ListDir(rel, func(name string, info littlefs.Info) bool {
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
	requestID, path, ok := proto.DecodeVFSMkdirPayload(msg.Payload())
	if !ok {
		_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSMkdir, 0, "decode mkdir")
		return
	}

	backend, rel, ok := s.resolve(path)
	if !ok {
		if isSDPath(path) {
			_ = s.sendErr(ctx, reply, proto.ErrNotFound, proto.MsgVFSMkdir, requestID, "sd not available")
		} else if s.fs == nil {
			_ = s.sendErr(ctx, reply, proto.ErrInternal, proto.MsgVFSMkdir, requestID, "vfs not ready")
		} else {
			_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSMkdir, requestID, "invalid path")
		}
		return
	}
	if err := backend.Mkdir(rel); err != nil {
		_ = s.sendErr(ctx, reply, mapVFSError(err), proto.MsgVFSMkdir, requestID, err.Error())
		return
	}
	_ = s.send(ctx, reply, proto.MsgVFSMkdirResp, proto.VFSMkdirRespPayload(requestID))
}

func (s *Service) handleRemove(ctx *kernel.Context, msg kernel.Message) {
	reply := msg.Cap
	requestID, path, ok := proto.DecodeVFSRemovePayload(msg.Payload())
	if !ok {
		_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSRemove, 0, "decode remove")
		return
	}

	backend, rel, ok := s.resolve(path)
	if !ok {
		if isSDPath(path) {
			_ = s.sendErr(ctx, reply, proto.ErrNotFound, proto.MsgVFSRemove, requestID, "sd not available")
		} else if s.fs == nil {
			_ = s.sendErr(ctx, reply, proto.ErrInternal, proto.MsgVFSRemove, requestID, "vfs not ready")
		} else {
			_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSRemove, requestID, "invalid path")
		}
		return
	}
	if err := backend.Remove(rel); err != nil {
		_ = s.sendErr(ctx, reply, mapVFSError(err), proto.MsgVFSRemove, requestID, err.Error())
		return
	}
	_ = s.send(ctx, reply, proto.MsgVFSRemoveResp, proto.VFSRemoveRespPayload(requestID))
}

func (s *Service) handleRename(ctx *kernel.Context, msg kernel.Message) {
	reply := msg.Cap
	requestID, oldPath, newPath, ok := proto.DecodeVFSRenamePayload(msg.Payload())
	if !ok {
		_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSRename, 0, "decode rename")
		return
	}

	oldFS, oldRel, ok := s.resolve(oldPath)
	if !ok {
		if isSDPath(oldPath) {
			_ = s.sendErr(ctx, reply, proto.ErrNotFound, proto.MsgVFSRename, requestID, "sd not available")
		} else if s.fs == nil {
			_ = s.sendErr(ctx, reply, proto.ErrInternal, proto.MsgVFSRename, requestID, "vfs not ready")
		} else {
			_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSRename, requestID, "invalid old path")
		}
		return
	}
	newFS, newRel, ok := s.resolve(newPath)
	if !ok {
		if isSDPath(newPath) {
			_ = s.sendErr(ctx, reply, proto.ErrNotFound, proto.MsgVFSRename, requestID, "sd not available")
		} else if s.fs == nil {
			_ = s.sendErr(ctx, reply, proto.ErrInternal, proto.MsgVFSRename, requestID, "vfs not ready")
		} else {
			_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSRename, requestID, "invalid new path")
		}
		return
	}
	if oldFS != newFS {
		_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSRename, requestID, "cross-device rename not supported")
		return
	}

	if err := oldFS.Rename(oldRel, newRel); err != nil {
		_ = s.sendErr(ctx, reply, mapVFSError(err), proto.MsgVFSRename, requestID, err.Error())
		return
	}
	_ = s.send(ctx, reply, proto.MsgVFSRenameResp, proto.VFSRenameRespPayload(requestID))
}

func (s *Service) handleCopy(ctx *kernel.Context, msg kernel.Message) {
	reply := msg.Cap
	requestID, srcPath, dstPath, ok := proto.DecodeVFSCopyPayload(msg.Payload())
	if !ok {
		_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSCopy, 0, "decode copy")
		return
	}
	if !reply.Valid() {
		return
	}

	srcFS, srcRel, ok := s.resolve(srcPath)
	if !ok {
		if isSDPath(srcPath) {
			_ = s.sendErr(ctx, reply, proto.ErrNotFound, proto.MsgVFSCopy, requestID, "sd not available")
		} else if s.fs == nil {
			_ = s.sendErr(ctx, reply, proto.ErrInternal, proto.MsgVFSCopy, requestID, "vfs not ready")
		} else {
			_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSCopy, requestID, "invalid src path")
		}
		return
	}
	dstFS, dstRel, ok := s.resolve(dstPath)
	if !ok {
		if isSDPath(dstPath) {
			_ = s.sendErr(ctx, reply, proto.ErrNotFound, proto.MsgVFSCopy, requestID, "sd not available")
		} else if s.fs == nil {
			_ = s.sendErr(ctx, reply, proto.ErrInternal, proto.MsgVFSCopy, requestID, "vfs not ready")
		} else {
			_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSCopy, requestID, "invalid dst path")
		}
		return
	}

	info, err := srcFS.Stat(srcRel)
	if err != nil {
		_ = s.sendErr(ctx, reply, mapVFSError(err), proto.MsgVFSCopy, requestID, err.Error())
		return
	}
	if info.Type != littlefs.TypeFile {
		_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSCopy, requestID, "source is not a file")
		return
	}

	w, err := dstFS.OpenWriter(dstRel, littlefs.WriteTruncate)
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
		n, eof, err := srcFS.ReadAt(srcRel, buf, off)
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
	requestID, path, ok := proto.DecodeVFSStatPayload(msg.Payload())
	if !ok {
		_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSStat, 0, "decode stat")
		return
	}

	backend, rel, ok := s.resolve(path)
	if !ok {
		if isSDPath(path) {
			_ = s.sendErr(ctx, reply, proto.ErrNotFound, proto.MsgVFSStat, requestID, "sd not available")
		} else if s.fs == nil {
			_ = s.sendErr(ctx, reply, proto.ErrInternal, proto.MsgVFSStat, requestID, "vfs not ready")
		} else {
			_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSStat, requestID, "invalid path")
		}
		return
	}
	if path == "/sd" && s.sd != nil {
		_ = s.send(ctx, reply, proto.MsgVFSStatResp, proto.VFSStatRespPayload(requestID, proto.VFSEntryDir, 0))
		return
	}

	info, err := backend.Stat(rel)
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
	requestID, path, off, maxBytes, ok := proto.DecodeVFSReadPayload(msg.Payload())
	if !ok {
		_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSRead, 0, "decode read")
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

	backend, rel, ok := s.resolve(path)
	if !ok {
		if isSDPath(path) {
			_ = s.sendErr(ctx, reply, proto.ErrNotFound, proto.MsgVFSRead, requestID, "sd not available")
		} else if s.fs == nil {
			_ = s.sendErr(ctx, reply, proto.ErrInternal, proto.MsgVFSRead, requestID, "vfs not ready")
		} else {
			_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSRead, requestID, "invalid path")
		}
		return
	}

	n, eof, err := backend.ReadAt(rel, buf, off)
	if err != nil {
		_ = s.sendErr(ctx, reply, mapVFSError(err), proto.MsgVFSRead, requestID, err.Error())
		return
	}

	_ = s.send(ctx, reply, proto.MsgVFSReadResp, proto.VFSReadRespPayload(requestID, off, eof, buf[:n]))
}

func (s *Service) handleWriteOpen(ctx *kernel.Context, msg kernel.Message) {
	reply := msg.Cap
	requestID, mode, path, ok := proto.DecodeVFSWriteOpenPayload(msg.Payload())
	if !ok {
		_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSWriteOpen, 0, "decode write open")
		return
	}
	if !reply.Valid() {
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

	backend, rel, ok := s.resolve(path)
	if !ok {
		if isSDPath(path) {
			_ = s.sendErr(ctx, reply, proto.ErrNotFound, proto.MsgVFSWriteOpen, requestID, "sd not available")
		} else if s.fs == nil {
			_ = s.sendErr(ctx, reply, proto.ErrInternal, proto.MsgVFSWriteOpen, requestID, "vfs not ready")
		} else {
			_ = s.sendErr(ctx, reply, proto.ErrBadMessage, proto.MsgVFSWriteOpen, requestID, "invalid path")
		}
		return
	}

	w, err := backend.OpenWriter(rel, wmode)
	if err != nil {
		_ = s.sendErr(ctx, reply, mapVFSError(err), proto.MsgVFSWriteOpen, requestID, err.Error())
		return
	}

	s.writers[requestID] = &writeSession{reply: reply, writer: w}
	_ = s.send(ctx, reply, proto.MsgVFSWriteResp, proto.VFSWriteRespPayload(requestID, false, 0))
}

func (s *Service) handleWriteChunk(ctx *kernel.Context, msg kernel.Message) {
	requestID, data, ok := proto.DecodeVFSWriteChunkPayload(msg.Payload())
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
	requestID, ok := proto.DecodeVFSWriteClosePayload(msg.Payload())
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
	res := ctx.SendToCapRetry(to, uint16(kind), payload, kernel.Capability{}, 500)
	if res == kernel.SendOK {
		return nil
	}
	return fmt.Errorf("vfs send %s: %s", kind, res)
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

func (s *Service) resolve(path string) (fsHandle, string, bool) {
	if path == "" {
		return nil, "", false
	}
	if path == "/sd" || path == "/sd/" {
		if s.sd == nil {
			return nil, "", false
		}
		return s.sd, "/", true
	}
	if strings.HasPrefix(path, "/sd/") {
		if s.sd == nil {
			return nil, "", false
		}
		return s.sd, path[3:], true
	}
	if s.fs == nil {
		return nil, "", false
	}
	root := flashFS{fs: s.fs}
	return root, path, true
}

func isSDPath(path string) bool {
	return path == "/sd" || path == "/sd/" || strings.HasPrefix(path, "/sd/")
}
