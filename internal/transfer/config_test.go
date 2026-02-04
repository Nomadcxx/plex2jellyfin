package transfer

import (
	"os/user"
	"strconv"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/config"
)

func TestOptionsFromConfig_NilConfig(t *testing.T) {
	opts := OptionsFromConfig(nil)

	defaults := DefaultOptions()

	if opts.Timeout != defaults.Timeout {
		t.Errorf("Timeout mismatch: got %v, want %v", opts.Timeout, defaults.Timeout)
	}
	if opts.TargetUID != defaults.TargetUID {
		t.Errorf("TargetUID mismatch: got %d, want %d", opts.TargetUID, defaults.TargetUID)
	}
	if opts.TargetGID != defaults.TargetGID {
		t.Errorf("TargetGID mismatch: got %d, want %d", opts.TargetGID, defaults.TargetGID)
	}
	if opts.FileMode != defaults.FileMode {
		t.Errorf("FileMode mismatch: got %v, want %v", opts.FileMode, defaults.FileMode)
	}
	if opts.DirMode != defaults.DirMode {
		t.Errorf("DirMode mismatch: got %v, want %v", opts.DirMode, defaults.DirMode)
	}
}

func TestOptionsFromConfig_EmptyPermissions(t *testing.T) {
	cfg := &config.Config{
		Permissions: config.PermissionsConfig{},
	}

	opts := OptionsFromConfig(cfg)
	defaults := DefaultOptions()

	// Should return defaults when no permissions are set
	if opts.TargetUID != defaults.TargetUID {
		t.Errorf("TargetUID mismatch: got %d, want %d", opts.TargetUID, defaults.TargetUID)
	}
	if opts.TargetGID != defaults.TargetGID {
		t.Errorf("TargetGID mismatch: got %d, want %d", opts.TargetGID, defaults.TargetGID)
	}
}

func TestOptionsFromConfig_NumericUIDGID(t *testing.T) {
	cfg := &config.Config{
		Permissions: config.PermissionsConfig{
			User:  "1000",
			Group: "1001",
		},
	}

	opts := OptionsFromConfig(cfg)

	if opts.TargetUID != 1000 {
		t.Errorf("TargetUID mismatch: got %d, want 1000", opts.TargetUID)
	}
	if opts.TargetGID != 1001 {
		t.Errorf("TargetGID mismatch: got %d, want 1001", opts.TargetGID)
	}
}

func TestOptionsFromConfig_CurrentUser(t *testing.T) {
	// Get current user info
	currentUser, err := user.Current()
	if err != nil {
		t.Skipf("Cannot get current user: %v", err)
	}

	currentGroup, err := user.LookupGroupId(currentUser.Gid)
	if err != nil {
		t.Skipf("Cannot get current group: %v", err)
	}

	cfg := &config.Config{
		Permissions: config.PermissionsConfig{
			User:  currentUser.Username,
			Group: currentGroup.Name,
		},
	}

	opts := OptionsFromConfig(cfg)

	expectedUID, _ := strconv.Atoi(currentUser.Uid)
	expectedGID, _ := strconv.Atoi(currentUser.Gid)

	if opts.TargetUID != expectedUID {
		t.Errorf("TargetUID mismatch: got %d, want %d", opts.TargetUID, expectedUID)
	}
	if opts.TargetGID != expectedGID {
		t.Errorf("TargetGID mismatch: got %d, want %d", opts.TargetGID, expectedGID)
	}
}

func TestOptionsFromConfig_FileAndDirModes(t *testing.T) {
	tests := []struct {
		name         string
		permissions  config.PermissionsConfig
		wantFileMode uint32
		wantDirMode  uint32
	}{
		{
			name: "file mode only (3 digit)",
			permissions: config.PermissionsConfig{
				FileMode: "644",
			},
			wantFileMode: 0644,
			wantDirMode:  0,
		},
		{
			name: "file mode only (4 digit)",
			permissions: config.PermissionsConfig{
				FileMode: "0644",
			},
			wantFileMode: 0644,
			wantDirMode:  0,
		},
		{
			name: "dir mode only (3 digit)",
			permissions: config.PermissionsConfig{
				DirMode: "755",
			},
			wantFileMode: 0,
			wantDirMode:  0755,
		},
		{
			name: "dir mode only (4 digit)",
			permissions: config.PermissionsConfig{
				DirMode: "0755",
			},
			wantFileMode: 0,
			wantDirMode:  0755,
		},
		{
			name: "both modes",
			permissions: config.PermissionsConfig{
				FileMode: "0640",
				DirMode:  "0750",
			},
			wantFileMode: 0640,
			wantDirMode:  0750,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Permissions: tt.permissions,
			}

			opts := OptionsFromConfig(cfg)

			if uint32(opts.FileMode) != tt.wantFileMode {
				t.Errorf("FileMode mismatch: got %04o, want %04o", opts.FileMode, tt.wantFileMode)
			}
			if uint32(opts.DirMode) != tt.wantDirMode {
				t.Errorf("DirMode mismatch: got %04o, want %04o", opts.DirMode, tt.wantDirMode)
			}
		})
	}
}

func TestOptionsFromConfig_InvalidValuesIgnored(t *testing.T) {
	cfg := &config.Config{
		Permissions: config.PermissionsConfig{
			User:     "nonexistentuser12345",
			Group:    "nonexistentgroup12345",
			FileMode: "invalid",
			DirMode:  "invalid",
		},
	}

	opts := OptionsFromConfig(cfg)
	defaults := DefaultOptions()

	// Should fall back to defaults when resolution fails
	if opts.TargetUID != defaults.TargetUID {
		t.Errorf("TargetUID should be default when user lookup fails: got %d, want %d", opts.TargetUID, defaults.TargetUID)
	}
	if opts.TargetGID != defaults.TargetGID {
		t.Errorf("TargetGID should be default when group lookup fails: got %d, want %d", opts.TargetGID, defaults.TargetGID)
	}
	if opts.FileMode != defaults.FileMode {
		t.Errorf("FileMode should be default when parse fails: got %v, want %v", opts.FileMode, defaults.FileMode)
	}
	if opts.DirMode != defaults.DirMode {
		t.Errorf("DirMode should be default when parse fails: got %v, want %v", opts.DirMode, defaults.DirMode)
	}
}

func TestOptionsFromConfig_FullConfiguration(t *testing.T) {
	cfg := &config.Config{
		Permissions: config.PermissionsConfig{
			User:     "1000",
			Group:    "1001",
			FileMode: "0644",
			DirMode:  "0755",
		},
	}

	opts := OptionsFromConfig(cfg)

	if opts.TargetUID != 1000 {
		t.Errorf("TargetUID mismatch: got %d, want 1000", opts.TargetUID)
	}
	if opts.TargetGID != 1001 {
		t.Errorf("TargetGID mismatch: got %d, want 1001", opts.TargetGID)
	}
	if opts.FileMode != 0644 {
		t.Errorf("FileMode mismatch: got %04o, want 0644", opts.FileMode)
	}
	if opts.DirMode != 0755 {
		t.Errorf("DirMode mismatch: got %04o, want 0755", opts.DirMode)
	}

	// Verify other options are still defaults
	defaults := DefaultOptions()
	if opts.Timeout != defaults.Timeout {
		t.Errorf("Timeout should be default: got %v, want %v", opts.Timeout, defaults.Timeout)
	}
	if opts.RetryAttempts != defaults.RetryAttempts {
		t.Errorf("RetryAttempts should be default: got %d, want %d", opts.RetryAttempts, defaults.RetryAttempts)
	}
}
