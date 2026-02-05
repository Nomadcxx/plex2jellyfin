package paths

import (
	"os"
	"os/user"
	"testing"
)

func TestUserHomeDir_NoSudo(t *testing.T) {
	// Clear SUDO_USER to simulate normal execution
	os.Unsetenv("SUDO_USER")

	got, err := UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() error = %v", err)
	}

	// Should return current user's home
	expected, _ := os.UserHomeDir()
	if got != expected {
		t.Errorf("UserHomeDir() = %q, want %q", got, expected)
	}
}

func TestUserHomeDir_WithSudoUser(t *testing.T) {
	// Get current user to use as test subject
	currentUser, err := user.Current()
	if err != nil {
		t.Skip("Cannot get current user")
	}

	// Set SUDO_USER to current user (simulates sudo from this user)
	os.Setenv("SUDO_USER", currentUser.Username)
	defer os.Unsetenv("SUDO_USER")

	got, err := UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() error = %v", err)
	}

	if got != currentUser.HomeDir {
		t.Errorf("UserHomeDir() = %q, want %q", got, currentUser.HomeDir)
	}
}

func TestUserHomeDir_SudoUserRoot(t *testing.T) {
	// SUDO_USER=root should be ignored
	os.Setenv("SUDO_USER", "root")
	defer os.Unsetenv("SUDO_USER")

	got, err := UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() error = %v", err)
	}

	// Should fall back to current user's home (not /root)
	expected, _ := os.UserHomeDir()
	if got != expected {
		t.Errorf("UserHomeDir() = %q, want %q", got, expected)
	}
}

func TestUserHomeDir_NonexistentUser(t *testing.T) {
	// SUDO_USER set to nonexistent user should fall back
	os.Setenv("SUDO_USER", "nonexistent_user_12345")
	defer os.Unsetenv("SUDO_USER")

	got, err := UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() error = %v", err)
	}

	// Should fall back to current user's home
	expected, _ := os.UserHomeDir()
	if got != expected {
		t.Errorf("UserHomeDir() = %q, want %q", got, expected)
	}
}

func TestUserConfigDir(t *testing.T) {
	os.Unsetenv("SUDO_USER")

	got, err := UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir() error = %v", err)
	}

	homeDir, _ := os.UserHomeDir()
	expected := homeDir + "/.config"
	if got != expected {
		t.Errorf("UserConfigDir() = %q, want %q", got, expected)
	}
}

func TestJellyWatchDir(t *testing.T) {
	os.Unsetenv("SUDO_USER")

	got, err := JellyWatchDir()
	if err != nil {
		t.Fatalf("JellyWatchDir() error = %v", err)
	}

	homeDir, _ := os.UserHomeDir()
	expected := homeDir + "/.config/jellywatch"
	if got != expected {
		t.Errorf("JellyWatchDir() = %q, want %q", got, expected)
	}
}

func TestDatabasePath(t *testing.T) {
	os.Unsetenv("SUDO_USER")

	got, err := DatabasePath()
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}

	homeDir, _ := os.UserHomeDir()
	expected := homeDir + "/.config/jellywatch/media.db"
	if got != expected {
		t.Errorf("DatabasePath() = %q, want %q", got, expected)
	}
}

func TestConfigPath(t *testing.T) {
	os.Unsetenv("SUDO_USER")

	got, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath() error = %v", err)
	}

	homeDir, _ := os.UserHomeDir()
	expected := homeDir + "/.config/jellywatch/config.toml"
	if got != expected {
		t.Errorf("ConfigPath() = %q, want %q", got, expected)
	}
}

func TestPlansDir(t *testing.T) {
	os.Unsetenv("SUDO_USER")

	got, err := PlansDir()
	if err != nil {
		t.Fatalf("PlansDir() error = %v", err)
	}

	homeDir, _ := os.UserHomeDir()
	expected := homeDir + "/.config/jellywatch/plans"
	if got != expected {
		t.Errorf("PlansDir() = %q, want %q", got, expected)
	}
}

func TestActualUser_NoSudo(t *testing.T) {
	os.Unsetenv("SUDO_USER")

	got := ActualUser()

	currentUser, _ := user.Current()
	if got != currentUser.Username {
		t.Errorf("ActualUser() = %q, want %q", got, currentUser.Username)
	}
}

func TestActualUser_WithSudo(t *testing.T) {
	os.Setenv("SUDO_USER", "testuser")
	defer os.Unsetenv("SUDO_USER")

	got := ActualUser()
	if got != "testuser" {
		t.Errorf("ActualUser() = %q, want %q", got, "testuser")
	}
}

func TestActualUser_SudoRoot(t *testing.T) {
	os.Setenv("SUDO_USER", "root")
	defer os.Unsetenv("SUDO_USER")

	got := ActualUser()

	// Should fall back to current user, not return "root"
	currentUser, _ := user.Current()
	if got != currentUser.Username {
		t.Errorf("ActualUser() = %q, want %q", got, currentUser.Username)
	}
}
