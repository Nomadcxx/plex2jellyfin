package fileidentity

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func NewQuarantinePath(path string) (string, error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".p2j-quarantine-*")
	if err != nil {
		return "", fmt.Errorf("reserve quarantine path beside %s: %w", path, err)
	}
	quarantine := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(quarantine)
		return "", fmt.Errorf("close quarantine placeholder: %w", err)
	}
	if err := os.Remove(quarantine); err != nil {
		return "", fmt.Errorf("release quarantine placeholder: %w", err)
	}
	return quarantine, nil
}

// MoveToQuarantine atomically takes ownership of the current path, then checks
// that it is still the captured object. A replacement is restored untouched.
func MoveToQuarantine(path, quarantine, identity string, noFollow bool) error {
	if identity == "" {
		return fmt.Errorf("cannot quarantine %s without a file identity", path)
	}
	if err := renameNoReplace(path, quarantine); err != nil {
		return fmt.Errorf("quarantine %s: %w", path, err)
	}
	if matches := matchesPath(quarantine, identity, noFollow); matches {
		return nil
	}
	actual, ok := identityAtPath(quarantine, noFollow)
	if !ok {
		return fmt.Errorf("quarantined object at %s changed and cannot be identified; leaving it in place", quarantine)
	}
	if err := RestoreQuarantine(quarantine, path, actual, noFollow); err != nil {
		return fmt.Errorf("object at %s changed; restore from %s failed: %w", path, quarantine, err)
	}
	return fmt.Errorf("file changed before quarantine: %s", path)
}

func RestoreQuarantine(quarantine, path, identity string, noFollow bool) error {
	if !matchesPath(quarantine, identity, noFollow) {
		return fmt.Errorf("quarantined file identity changed: %s", quarantine)
	}
	if err := renameNoReplace(quarantine, path); err != nil {
		return fmt.Errorf("restore %s to %s: %w", quarantine, path, err)
	}
	return nil
}

func RemoveQuarantine(quarantine, identity string, noFollow bool) error {
	if !matchesPath(quarantine, identity, noFollow) {
		return fmt.Errorf("quarantined file identity changed: %s", quarantine)
	}
	if err := os.Remove(quarantine); err != nil {
		return fmt.Errorf("remove quarantined file %s: %w", quarantine, err)
	}
	return nil
}

func RemoveIfMatches(path, identity string, noFollow bool) error {
	quarantine, err := NewQuarantinePath(path)
	if err != nil {
		return err
	}
	if err := MoveToQuarantine(path, quarantine, identity, noFollow); err != nil {
		return err
	}
	if err := RemoveQuarantine(quarantine, identity, noFollow); err != nil {
		return errors.Join(err, RestoreQuarantine(quarantine, path, identity, noFollow))
	}
	return nil
}

func matchesPath(path, identity string, noFollow bool) bool {
	if noFollow {
		return MatchesLstat(path, identity)
	}
	return MatchesPath(path, identity)
}

func identityAtPath(path string, noFollow bool) (string, bool) {
	if noFollow {
		return FromLstat(path)
	}
	return FromPath(path)
}
