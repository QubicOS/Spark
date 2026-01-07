package hal

type nullAudio struct{}

func (nullAudio) PWM() PWMAudio { return nil }
