package shell

import "spark/sparkos/internal/userdb"

func (s *Service) applyUser(rec userdb.Record) {
	s.user = rec.Name
	s.userRole = rec.Role
	s.userHome = rec.Home

	if s.userHome != "" {
		s.cwd = s.userHome
	}
	if s.cwd == "" {
		s.cwd = "/"
	}

	if s.tabIdx >= 0 && s.tabIdx < len(s.tabs) {
		s.tabs[s.tabIdx].user = s.user
		s.tabs[s.tabIdx].userRole = s.userRole
		s.tabs[s.tabIdx].userHome = s.userHome
		s.tabs[s.tabIdx].cwd = s.cwd
	}
}
