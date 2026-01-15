package shell

import (
	"errors"
	"strings"

	"spark/sparkos/internal/userdb"
	"spark/sparkos/kernel"
)

func registerUserCommands(r *registry) error {
	for _, cmd := range []command{
		{Name: "whoami", Usage: "whoami", Desc: "Print current user.", Run: cmdWhoami},
		{Name: "su", Usage: "su [user]", Desc: "Switch user (prompts for password).", Run: cmdSu},
	} {
		if err := r.register(cmd); err != nil {
			return err
		}
	}
	return nil
}

func cmdWhoami(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	if len(args) != 0 {
		return errors.New("usage: whoami")
	}
	u := strings.TrimSpace(s.user)
	if u == "" {
		u = "unknown"
	}
	return s.printString(ctx, u+"\n")
}

func cmdSu(ctx *kernel.Context, s *Service, args []string, _ redirection) error {
	if len(args) > 1 {
		return errors.New("usage: su [user]")
	}
	target := "root"
	if len(args) == 1 && strings.TrimSpace(args[0]) != "" {
		target = strings.TrimSpace(args[0])
	}
	if target == s.user {
		return nil
	}

	users, ok, err := s.loadUsers(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("su: users db missing")
	}
	rec, found := userdb.Find(users, target)
	if !found {
		return errors.New("su: unknown user")
	}

	if s.userRole == userdb.RoleAdmin {
		s.applyUser(rec)
		_ = s.printString(ctx, s.tabStatusLine())
		return nil
	}

	s.suppressPromptOnce = true
	s.beginSu(ctx, target)
	return nil
}
