package dogeboxd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Supported config field types for pup manifests.
// These are the canonical types that match the frontend form renderer.
var supportedConfigTypes = map[string]struct{}{
	"text":     {}, // plain text input
	"password": {}, // password/secret input
	"number":   {}, // numeric input
	"toggle":   {}, // toggle switch
	"email":    {}, // email input with validation
	"textarea": {}, // multi-line text input
	"select":   {}, // dropdown selection
	"checkbox": {}, // checkbox input
	"radio":    {}, // radio button group
	"date":     {}, // date picker
	"range":    {}, // slider input
	"color":    {}, // color picker
}

// ManifestConfigFieldIndex returns a map of field name to field definition.
func ManifestConfigFieldIndex(cfg PupManifestConfigFields) map[string]PupManifestConfigField {
	index := make(map[string]PupManifestConfigField)
	for _, section := range cfg.Sections {
		for _, field := range section.Fields {
			index[field.Name] = field
		}
	}
	return index
}

// ExtractManifestConfigDefaults converts configured defaults into the string representation used for storage.
func ExtractManifestConfigDefaults(cfg PupManifestConfigFields) (map[string]string, error) {
	index := ManifestConfigFieldIndex(cfg)
	defaults := make(map[string]string, len(index))

	for name, field := range index {
		if field.Default == nil {
			continue
		}

		strValue, err := stringifyConfigValue(field.Type, field.Default)
		if err != nil {
			return nil, fmt.Errorf("config field %s default: %w", name, err)
		}
		defaults[name] = strValue
	}

	return defaults, nil
}

// ManifestConfigNeedsValues determines if any required configuration values are missing.
func ManifestConfigNeedsValues(cfg PupManifestConfigFields, values map[string]string) bool {
	if len(cfg.Sections) == 0 {
		return false
	}

	for _, section := range cfg.Sections {
		for _, field := range section.Fields {
			if !field.Required {
				continue
			}
			val, ok := values[field.Name]
			if !ok {
				return true
			}

			if strings.TrimSpace(val) == "" {
				return true
			}
		}
	}

	return false
}

// CoerceConfigPayload normalizes incoming configuration payloads into string representations.
func CoerceConfigPayload(cfg PupManifestConfigFields, payload map[string]any) (map[string]string, error) {
	fieldIndex := ManifestConfigFieldIndex(cfg)
	normalized := make(map[string]string, len(payload))

	for key, raw := range payload {
		field, ok := fieldIndex[key]
		if !ok {
			// ignore unknown keys silently to allow forwards compatibility
			continue
		}

		value, err := stringifyConfigValue(field.Type, raw)
		if err != nil {
			return nil, fmt.Errorf("config field %s: %w", key, err)
		}
		normalized[key] = value
	}

	return normalized, nil
}

// stringifyConfigValue converts a raw config value to its string representation based on field type.
func stringifyConfigValue(fieldType string, raw any) (string, error) {
	if raw == nil {
		return "", nil
	}

	switch fieldType {
	case "text", "password", "email", "textarea", "date", "color", "select", "radio":
		switch v := raw.(type) {
		case string:
			return v, nil
		default:
			return fmt.Sprintf("%v", raw), nil
		}

	case "number", "range":
		switch v := raw.(type) {
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64), nil
		case float32:
			return strconv.FormatFloat(float64(v), 'f', -1, 64), nil
		case int:
			return strconv.Itoa(v), nil
		case int64:
			return strconv.FormatInt(v, 10), nil
		case uint64:
			return strconv.FormatUint(v, 10), nil
		case json.Number:
			return v.String(), nil
		case string:
			if v == "" {
				return "", nil
			}
			if _, err := strconv.ParseFloat(v, 64); err != nil {
				return "", fmt.Errorf("expected number, got %q", v)
			}
			return v, nil
		default:
			return "", fmt.Errorf("expected number, got %T", raw)
		}

	case "toggle", "checkbox":
		switch v := raw.(type) {
		case bool:
			return strconv.FormatBool(v), nil
		case string:
			switch strings.ToLower(strings.TrimSpace(v)) {
			case "true", "1", "yes", "on":
				return "true", nil
			case "false", "0", "no", "off":
				return "false", nil
			default:
				return "", fmt.Errorf("expected boolean, got %q", v)
			}
		default:
			return "", fmt.Errorf("expected boolean, got %T", raw)
		}

	default:
		if _, ok := supportedConfigTypes[fieldType]; !ok {
			return "", fmt.Errorf("unsupported field type %s", fieldType)
		}
		return fmt.Sprintf("%v", raw), nil
	}
}
