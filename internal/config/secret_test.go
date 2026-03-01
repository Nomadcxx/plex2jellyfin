package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateWebhookSecret(t *testing.T) {
	secret, err := GenerateWebhookSecret()
	require.NoError(t, err)
	assert.Len(t, secret, 64)

	secret2, err := GenerateWebhookSecret()
	require.NoError(t, err)
	assert.NotEqual(t, secret, secret2)
}
