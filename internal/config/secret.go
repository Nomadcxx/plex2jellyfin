package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// GenerateWebhookSecret returns a 32-byte random secret encoded as hex.
func GenerateWebhookSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating webhook secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}
