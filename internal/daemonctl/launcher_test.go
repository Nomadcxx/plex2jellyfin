package daemonctl

import (
	"errors"
	"testing"
)

func TestLauncherSelectsSystemdUserWhenAvailable(t *testing.T) {
	started := false
	l := &Launcher{
		systemdUserExists: func() bool { return true },
		systemdUserStart:  func() error { started = true; return nil },
	}
	if err := l.Start(); err != nil {
		t.Fatal(err)
	}
	if !started {
		t.Fatal("expected user start")
	}
}

func TestLauncherEnableSystemUnit(t *testing.T) {
	enabled := false
	l := &Launcher{
		systemdUserExists:   func() bool { return false },
		systemdSystemExists: func() bool { return true },
		systemdSystemEnable: func() error { enabled = true; return nil },
	}
	if err := l.Enable(); err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Fatal("expected system enable")
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
	if err := l.Enable(); err != nil {
		t.Fatal(err)
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
