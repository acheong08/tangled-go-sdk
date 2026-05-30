package tangled

import "encoding/json"

// jsonUnmarshal unmarshals JSON data.
func jsonUnmarshal(b []byte, v any) error {
	return json.Unmarshal(b, v)
}

// jsonMarshal marshals data to JSON.
func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// jsonStr extracts a string field from a map.
func jsonStr(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

// jsonFloat extracts a float64 field from a map.
func jsonFloat(m map[string]any, key string) float64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	f, ok := v.(float64)
	if !ok {
		return 0
	}
	return f
}
