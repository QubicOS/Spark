package shell

import (
	"fmt"
	"sort"
	"strings"

	"spark/sparkos/kernel"
)

type cmdFunc func(ctx *kernel.Context, s *Service, args []string, redir redirection) error

type command struct {
	Name    string
	Aliases []string
	Usage   string
	Desc    string
	Run     cmdFunc
}

type registry struct {
	primary map[string]command
	lookup  map[string]string
}

func newRegistry() *registry {
	return &registry{
		primary: make(map[string]command),
		lookup:  make(map[string]string),
	}
}

func (r *registry) register(cmd command) error {
	cmd.Name = strings.TrimSpace(cmd.Name)
	if cmd.Name == "" {
		return fmt.Errorf("shell registry: empty command name")
	}
	if cmd.Run == nil {
		return fmt.Errorf("shell registry: %q has no handler", cmd.Name)
	}
	if _, ok := r.primary[cmd.Name]; ok {
		return fmt.Errorf("shell registry: duplicate command %q", cmd.Name)
	}

	r.primary[cmd.Name] = cmd
	r.lookup[cmd.Name] = cmd.Name

	for _, alias := range cmd.Aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		if _, ok := r.lookup[alias]; ok {
			return fmt.Errorf("shell registry: duplicate alias %q", alias)
		}
		r.lookup[alias] = cmd.Name
	}
	return nil
}

func (r *registry) resolve(name string) (command, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return command{}, false
	}
	if primary, ok := r.lookup[name]; ok {
		cmd, ok := r.primary[primary]
		return cmd, ok
	}
	return command{}, false
}

func (r *registry) names() []string {
	out := make([]string, 0, len(r.primary))
	for name := range r.primary {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (r *registry) matches(prefix string) []string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return nil
	}

	var out []string
	for _, name := range r.names() {
		if strings.HasPrefix(name, prefix) {
			out = append(out, name)
		}
	}
	return out
}
