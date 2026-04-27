package daemonctl

// Launcher resolves a daemon-start strategy. Task 3.2 expands this with
// systemd-user → systemd-system → detached-exec resolution.
type Launcher struct {
	BinaryPath string
	LogPath    string
}

func (l *Launcher) Start() error {
	return nil
}
