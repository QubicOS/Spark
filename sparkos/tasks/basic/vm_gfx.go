package basic

func (m *vm) execCLS(_ *scanner) (stepResult, error) {
	if m.fb == nil {
		return stepResult{}, nil
	}
	m.fb.ClearRGB(0, 0, 0)
	_ = m.fb.Present()
	return stepResult{}, nil
}
