//go:build debug

package shell

import "spark/sparkos/kernel"

func registerDebugCommands(r *registry) error {
	return r.register(command{
		Name:  "panic",
		Usage: "panic",
		Desc:  "Panic the shell task (debug/test).",
		Run:   cmdPanic,
	})
}

func cmdPanic(_ *kernel.Context, _ *Service, _ []string, _ redirection) error {
	panic("shell panic")
}
