package config

import "strings"

// DisplayValue returns the display string for a config key's raw value,
// handling masking of sensitive keys and falling back to defaults.
func DisplayValue(key, raw string) string {
	if IsSensitive(key) {
		if raw != "" {
			return maskSecret(raw)
		}
		return "(not set)"
	}
	if raw != "" {
		return raw
	}
	if d, ok := Defaults[key]; ok {
		return d
	}
	return "(not set)"
}

// maskSecret preserves a recognised prefix (e.g. "admp_") and masks the
// remainder with asterisks of matching length so the redacted form hints at
// the original length without leaking the secret.
func maskSecret(raw string) string {
	if idx := strings.Index(raw, "_"); idx >= 0 && idx < len(raw)-1 {
		return raw[:idx+1] + strings.Repeat("*", len(raw)-idx-1)
	}
	return strings.Repeat("*", len(raw))
}