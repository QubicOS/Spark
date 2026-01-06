package shell

import (
	"path"
	"strings"
)

func (s *Service) absPath(p string) string {
	if p == "" {
		return s.cwd
	}
	if strings.HasPrefix(p, "/") {
		return cleanPath(p)
	}
	if s.cwd == "/" {
		return cleanPath("/" + p)
	}
	return cleanPath(s.cwd + "/" + p)
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
