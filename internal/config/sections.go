package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/mitchellh/mapstructure"
)

type sectionAccessor struct {
	get func(c *Config) any
	set func(c *Config, raw json.RawMessage) error
}

var sections = map[string]sectionAccessor{
	"paths":       {get: func(c *Config) any { return c.Watch }, set: setWatch},
	"libraries":   {get: func(c *Config) any { return c.Libraries }, set: setLibraries},
	"sonarr":      {get: func(c *Config) any { return c.Sonarr }, set: setSonarr},
	"radarr":      {get: func(c *Config) any { return c.Radarr }, set: setRadarr},
	"jellyfin":    {get: func(c *Config) any { return c.Jellyfin }, set: setJellyfin},
	"jellystat":   {get: func(c *Config) any { return c.Jellystat }, set: setJellystat},
	"tmdb":        {get: func(c *Config) any { return c.TMDB }, set: setTMDB},
	"ai":          {get: func(c *Config) any { return c.AI }, set: setAI},
	"daemon":      {get: func(c *Config) any { return c.Daemon }, set: setDaemon},
	"logging":     {get: func(c *Config) any { return c.Logging }, set: setLogging},
	"options":     {get: func(c *Config) any { return c.Options }, set: setOptions},
	"permissions": {get: func(c *Config) any { return c.Permissions }, set: setPermissions},
}

func SectionNames() []string {
	out := make([]string, 0, len(sections))
	for name := range sections {
		out = append(out, name)
	}
	return out
}

func GetSection(c *Config, name string) (json.RawMessage, error) {
	accessor, ok := sections[name]
	if !ok {
		return nil, fmt.Errorf("unknown section %q", name)
	}
	return json.Marshal(toTaggedMap(reflect.ValueOf(accessor.get(c))))
}

func SetSection(c *Config, name string, raw json.RawMessage) error {
	accessor, ok := sections[name]
	if !ok {
		return fmt.Errorf("unknown section %q", name)
	}
	return accessor.set(c, raw)
}

func decodeSection(raw json.RawMessage, out any) error {
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName:          "mapstructure",
		Result:           out,
		WeaklyTypedInput: true,
	})
	if err != nil {
		return err
	}
	return dec.Decode(data)
}

func setWatch(c *Config, raw json.RawMessage) error {
	var v WatchConfig
	if err := decodeSection(raw, &v); err != nil {
		return err
	}
	c.Watch = v
	return nil
}

func setLibraries(c *Config, raw json.RawMessage) error {
	var v LibrariesConfig
	if err := decodeSection(raw, &v); err != nil {
		return err
	}
	c.Libraries = v
	return nil
}

func setSonarr(c *Config, raw json.RawMessage) error {
	var v SonarrConfig
	if err := decodeSection(raw, &v); err != nil {
		return err
	}
	c.Sonarr = v
	return nil
}

func setRadarr(c *Config, raw json.RawMessage) error {
	var v RadarrConfig
	if err := decodeSection(raw, &v); err != nil {
		return err
	}
	c.Radarr = v
	return nil
}

func setJellyfin(c *Config, raw json.RawMessage) error {
	var v JellyfinConfig
	if err := decodeSection(raw, &v); err != nil {
		return err
	}
	c.Jellyfin = v
	return nil
}

func setJellystat(c *Config, raw json.RawMessage) error {
	var v JellystatConfig
	if err := decodeSection(raw, &v); err != nil {
		return err
	}
	c.Jellystat = v
	return nil
}

func setTMDB(c *Config, raw json.RawMessage) error {
	var v TMDBConfig
	if err := decodeSection(raw, &v); err != nil {
		return err
	}
	c.TMDB = v
	return nil
}

func setAI(c *Config, raw json.RawMessage) error {
	var v AIConfig
	if err := decodeSection(raw, &v); err != nil {
		return err
	}
	c.AI = v
	return nil
}

func setDaemon(c *Config, raw json.RawMessage) error {
	var v DaemonConfig
	if err := decodeSection(raw, &v); err != nil {
		return err
	}
	c.Daemon = v
	return nil
}

func setLogging(c *Config, raw json.RawMessage) error {
	var v LoggingConfig
	if err := decodeSection(raw, &v); err != nil {
		return err
	}
	c.Logging = v
	return nil
}

func setOptions(c *Config, raw json.RawMessage) error {
	var v OptionsConfig
	if err := decodeSection(raw, &v); err != nil {
		return err
	}
	c.Options = v
	return nil
}

func setPermissions(c *Config, raw json.RawMessage) error {
	var v PermissionsConfig
	if err := decodeSection(raw, &v); err != nil {
		return err
	}
	c.Permissions = v
	return nil
}

func toTaggedMap(v reflect.Value) any {
	if !v.IsValid() {
		return nil
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Struct:
		out := make(map[string]any)
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" {
				continue
			}
			name := field.Tag.Get("mapstructure")
			if name == "" {
				name = strings.ToLower(field.Name)
			}
			if name == "-" {
				continue
			}
			out[name] = toTaggedMap(v.Field(i))
		}
		return out
	case reflect.Slice:
		out := make([]any, v.Len())
		for i := 0; i < v.Len(); i++ {
			out[i] = toTaggedMap(v.Index(i))
		}
		return out
	default:
		return v.Interface()
	}
}
