//go:build !debug

package shell

func registerDebugCommands(_ *registry) error {
	return nil
}
