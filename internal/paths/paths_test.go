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
