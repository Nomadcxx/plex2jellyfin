package daemonctl

import (
	"errors"
	"testing"
)

func TestLauncherSelectsSystemdUserWhenAvailable(t *testing.T) {
	l := &Launcher{
		systemdUserExists: func() bool { return true },
		systemdUserStart:  func() error { return nil },
	}
	if err := l.Start(); err != nil {
		t.Fatal(err)
	}
}

func TestLauncherFallsBackToDirectExec(t *testing.T) {
	called := false
	l := &Launcher{
		systemdUserExists:   func() bool { return false },
		systemdSystemExists: func() bool { return false },
		directExec:          func() error { called = true; return nil },
	}
	if err := l.Start(); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("direct exec not invoked")
	}
}

func TestLauncherReturnsStrategyError(t *testing.T) {
	l := &Launcher{
		systemdUserExists: func() bool { return true },
		systemdUserStart:  func() error { return errors.New("boom") },
	}
	if err := l.Start(); err == nil {
		t.Error("expected error")
	}
}
