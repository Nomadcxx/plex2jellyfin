package transfer

import (
	"github.com/Nomadcxx/jellywatch/internal/config"
)

// OptionsFromConfig creates TransferOptions from configuration.
// It starts with DefaultOptions() and applies any permission settings from cfg.
// If cfg is nil, it returns the default options unchanged.
func OptionsFromConfig(cfg *config.Config) TransferOptions {
	opts := DefaultOptions()

	if cfg == nil {
		return opts
	}

	// Apply ownership settings if configured
	if cfg.Permissions.WantsOwnership() {
		if uid, err := cfg.Permissions.ResolveUID(); err == nil && uid >= 0 {
			opts.TargetUID = uid
		}
		if gid, err := cfg.Permissions.ResolveGID(); err == nil && gid >= 0 {
			opts.TargetGID = gid
		}
	}

	// Apply mode settings if configured
	if cfg.Permissions.WantsMode() {
		if mode, err := cfg.Permissions.ParseFileMode(); err == nil && mode != 0 {
			opts.FileMode = mode
		}
		if mode, err := cfg.Permissions.ParseDirMode(); err == nil && mode != 0 {
			opts.DirMode = mode
		}
	}

	return opts
}
