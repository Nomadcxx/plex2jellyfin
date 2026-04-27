package config

import (
	"reflect"
	"testing"
)

func TestSecretFieldsTagged(t *testing.T) {
	want := map[string][]string{
		"SonarrConfig":   {"APIKey"},
		"RadarrConfig":   {"APIKey"},
		"JellyfinConfig": {"APIKey", "WebhookSecret", "PluginSharedSecret"},
		"Config":         {"Password"},
	}
	for typeName, fields := range want {
		typ := lookupConfigType(typeName)
		if typ == nil {
			t.Fatalf("type %s not found", typeName)
		}
		for _, f := range fields {
			sf, ok := typ.FieldByName(f)
			if !ok {
				t.Errorf("%s.%s missing", typeName, f)
				continue
			}
			if sf.Tag.Get("secret") != "true" {
				t.Errorf("%s.%s missing secret:\"true\" tag", typeName, f)
			}
		}
	}
}

func TestMaskSecretsMasksAPIKey(t *testing.T) {
	c := SonarrConfig{APIKey: "abcdef1234567890"}
	MaskSecrets(&c)
	if c.APIKey != "****7890" {
		t.Errorf("got %q", c.APIKey)
	}
}

func lookupConfigType(name string) reflect.Type {
	switch name {
	case "SonarrConfig":
		return reflect.TypeOf(SonarrConfig{})
	case "RadarrConfig":
		return reflect.TypeOf(RadarrConfig{})
	case "JellyfinConfig":
		return reflect.TypeOf(JellyfinConfig{})
	case "Config":
		return reflect.TypeOf(Config{})
	}
	return nil
}
