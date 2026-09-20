package config

// DisplayValue returns the display string for a config key's raw value,
// falling back to the key's default. Nothing in config.json is secret
// (credentials live in credentials.json), so values are shown as stored.
func DisplayValue(key, raw string) string {
	if raw != "" {
		return raw
	}
	if d, ok := Defaults[key]; ok {
		return d
	}
	return "(not set)"
}
