// Package privilege provides automatic sudo escalation for commands requiring root.
package privilege

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// NeedsRoot returns true if the current process is not running as root.
func NeedsRoot() bool {
	if os.Getenv("JELLYWATCH_TEST_NO_ESCALATE") == "1" {
		return false
	}
	return os.Geteuid() != 0
}

// Escalate re-executes the current command with sudo.
// The reason parameter explains why root is needed (e.g., "delete files and modify ownership").
// On success, this function does not return - the process is replaced by sudo.
// On failure (e.g., sudo not found), returns an error.
func Escalate(reason string) error {
	sudoPath, err := exec.LookPath("sudo")
	if err != nil {
		return fmt.Errorf("sudo not found in PATH: %w", err)
	}

	fmt.Printf("Root privileges required to %s.\n", reason)
	fmt.Println("Requesting sudo access...")
	fmt.Println()

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine executable path: %w", err)
	}

	args := append([]string{"sudo", exe}, os.Args[1:]...)

	return syscall.Exec(sudoPath, args, os.Environ())
}
