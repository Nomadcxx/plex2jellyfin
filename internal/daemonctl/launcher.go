package daemonctl

import (
	"errors"
	"os/exec"
	"syscall"
)

type Launcher struct {
	BinaryPath string
	LogPath    string

	systemdUserExists   func() bool
	systemdUserStart    func() error
	systemdUserEnable   func() error
	systemdSystemExists func() bool
	systemdSystemStart  func() error
	systemdSystemEnable func() error
	directExec          func() error
}

func New(binary, logPath string) *Launcher {
	l := &Launcher{BinaryPath: binary, LogPath: logPath}
	l.systemdUserExists = func() bool {
		return exec.Command("systemctl", "--user", "list-unit-files", "plex2jellyfin-daemon.service").Run() == nil
	}
	l.systemdUserStart = func() error {
		return exec.Command("systemctl", "--user", "enable", "--now", "plex2jellyfin-daemon").Run()
	}
	l.systemdUserEnable = func() error {
		return exec.Command("systemctl", "--user", "enable", "plex2jellyfin-daemon").Run()
	}
	l.systemdSystemExists = func() bool {
		return exec.Command("systemctl", "list-unit-files", "plex2jellyfin-daemon.service").Run() == nil
	}
	l.systemdSystemStart = func() error {
		return exec.Command("systemctl", "enable", "--now", "plex2jellyfin-daemon").Run()
	}
	l.systemdSystemEnable = func() error {
		return exec.Command("systemctl", "enable", "plex2jellyfin-daemon").Run()
	}
	l.directExec = func() error {
		f, err := openLogFile(logPath)
		if err != nil {
			return err
		}
		cmd := exec.Command(binary)
		cmd.Stdout, cmd.Stderr = f, f
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := cmd.Start(); err != nil {
			f.Close()
			return err
		}
		return nil
	}
	return l
}

// Enable marks the systemd unit for boot when a unit exists. No-op for direct-exec.
func (l *Launcher) Enable() error {
	if l.systemdUserExists != nil && l.systemdUserExists() {
		if l.systemdUserEnable != nil {
			return l.systemdUserEnable()
		}
		return nil
	}
	if l.systemdSystemExists != nil && l.systemdSystemExists() {
		if l.systemdSystemEnable != nil {
			return l.systemdSystemEnable()
		}
		return nil
	}
	return nil
}

func (l *Launcher) Start() error {
	if l.systemdUserExists != nil && l.systemdUserExists() {
		return l.systemdUserStart()
	}
	if l.systemdSystemExists != nil && l.systemdSystemExists() {
		return l.systemdSystemStart()
	}
	if l.directExec != nil {
		return l.directExec()
	}
	return errors.New("no daemon launch strategy available")
}
