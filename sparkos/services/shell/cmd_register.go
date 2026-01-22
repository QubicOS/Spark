package shell

func (s *Service) initRegistry() error {
	r := newRegistry()

	for _, register := range []func(r *registry) error{
		registerCoreCommands,
		registerDebugCommands,
		registerSysCommands,
		registerFSCommands,
		registerTextCommands,
		registerAppCommands,
		registerUserCommands,
	} {
		if err := register(r); err != nil {
			return err
		}
	}

	s.reg = r
	return nil
}

func (s *Service) initRegistryMinimal() error {
	r := newRegistry()
	for _, register := range []func(r *registry) error{
		registerCoreCommands,
		registerTextCommands,
	} {
		if err := register(r); err != nil {
			return err
		}
	}
	s.reg = r
	return nil
}
