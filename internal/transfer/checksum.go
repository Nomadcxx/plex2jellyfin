package transfer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

func verifyChecksum(src, dst string) (string, error) {
	sourceHash, err := checksumFile(src)
	if err != nil {
		return "", fmt.Errorf("checksum source: %w", err)
	}
	destinationHash, err := checksumFile(dst)
	if err != nil {
		return "", fmt.Errorf("checksum destination: %w", err)
	}
	if sourceHash != destinationHash {
		return "", fmt.Errorf("%w: source %s destination %s", ErrChecksumMismatch, sourceHash, destinationHash)
	}
	return sourceHash, nil
}

func checksumFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
