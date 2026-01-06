package shell

import vfsclient "spark/sparkos/client/vfs"

func (s *Service) vfsClient() *vfsclient.Client {
	if s.vfs == nil {
		s.vfs = vfsclient.New(s.vfsCap)
	}
	return s.vfs
}
