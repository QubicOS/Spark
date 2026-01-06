package mc

import (
	"path"
	"sort"
	"strings"

	vfsclient "spark/sparkos/client/vfs"
	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

type entry struct {
	Name     string
	Type     proto.VFSEntryType
	Size     uint32
	FullPath string
}

func (e entry) isDir() bool { return e.Type == proto.VFSEntryDir }

type panel struct {
	path string

	entries []entry
	sel     int
	scroll  int
}

func (p *panel) selected() (entry, bool) {
	if p.sel < 0 || p.sel >= len(p.entries) {
		return entry{}, false
	}
	return p.entries[p.sel], true
}

func (p *panel) setPath(dir string) {
	p.path = cleanPath(dir)
	if p.path == "" {
		p.path = "/"
	}
}

func (p *panel) up() {
	if p.sel > 0 {
		p.sel--
	}
}

func (p *panel) down() {
	if p.sel+1 < len(p.entries) {
		p.sel++
	}
}

func (p *panel) clamp(viewRows int) {
	if len(p.entries) == 0 {
		p.sel = 0
		p.scroll = 0
		return
	}
	if p.sel < 0 {
		p.sel = 0
	}
	if p.sel >= len(p.entries) {
		p.sel = len(p.entries) - 1
	}
	if viewRows <= 0 {
		p.scroll = 0
		return
	}
	if p.scroll < 0 {
		p.scroll = 0
	}
	if p.sel < p.scroll {
		p.scroll = p.sel
	}
	if p.sel >= p.scroll+viewRows {
		p.scroll = p.sel - viewRows + 1
	}
	maxScroll := len(p.entries) - viewRows
	if maxScroll < 0 {
		maxScroll = 0
	}
	if p.scroll > maxScroll {
		p.scroll = maxScroll
	}
}

func (t *Task) vfsClient() *vfsclient.Client {
	if t.vfs == nil {
		t.vfs = vfsclient.New(t.vfsCap)
	}
	return t.vfs
}

func (t *Task) loadDir(ctx *kernel.Context, p *panel) error {
	ents, err := t.vfsClient().List(ctx, p.path)
	if err != nil {
		return err
	}

	entries := make([]entry, 0, len(ents)+1)
	if p.path != "/" {
		parent := path.Dir(p.path)
		if parent == "." {
			parent = "/"
		}
		entries = append(entries, entry{
			Name:     "..",
			Type:     proto.VFSEntryDir,
			FullPath: parent,
		})
	}
	for _, e := range ents {
		full := joinPath(p.path, e.Name)
		entries = append(entries, entry{
			Name:     e.Name,
			Type:     e.Type,
			Size:     e.Size,
			FullPath: full,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		a := entries[i]
		b := entries[j]
		if a.Name == ".." {
			return true
		}
		if b.Name == ".." {
			return false
		}
		da := a.isDir()
		db := b.isDir()
		if da != db {
			return da
		}
		return a.Name < b.Name
	})

	p.entries = entries
	if p.sel >= len(p.entries) {
		p.sel = len(p.entries) - 1
	}
	if p.sel < 0 {
		p.sel = 0
	}
	return nil
}

func cleanPath(p string) string {
	p = path.Clean(p)
	if p == "." {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

func joinPath(dir, name string) string {
	if dir == "" || dir == "/" {
		return cleanPath("/" + name)
	}
	return cleanPath(dir + "/" + name)
}
