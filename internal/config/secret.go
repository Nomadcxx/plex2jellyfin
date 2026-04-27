package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"reflect"
)

// GenerateWebhookSecret returns a 32-byte random secret encoded as hex.
func GenerateWebhookSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating webhook secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func MaskSecrets(v any) {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() || rv.Kind() != reflect.Pointer || rv.IsNil() {
		return
	}
	maskValue(rv.Elem())
}

func maskValue(rv reflect.Value) {
	if rv.Kind() != reflect.Struct {
		return
	}
	rt := rv.Type()
	for i := 0; i < rv.NumField(); i++ {
		field := rv.Field(i)
		ft := rt.Field(i)
		if ft.Tag.Get("secret") == "true" && field.Kind() == reflect.String && field.CanSet() {
			field.SetString(maskSecret(field.String()))
			continue
		}
		if field.Kind() == reflect.Struct && field.CanAddr() {
			maskValue(field)
		}
	}
}

func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return "****"
	}
	return "****" + s[len(s)-4:]
}
