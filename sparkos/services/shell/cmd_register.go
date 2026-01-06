package shell

func (s *Service) initRegistry() error {
	r := newRegistry()

	for _, register := range []func(r *registry) error{
		registerCoreCommands,
		registerSysCommands,
		registerFSCommands,
		registerTextCommands,
		registerAppCommands,
	} {
		if err := register(r); err != nil {
			return err
		}
	}

	s.reg = r
	return nil
}
