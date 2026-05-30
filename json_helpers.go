package tangled

// jsonStr extracts a string field from a map[string]any.
func jsonStr(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

// jsonFloat extracts a float64 field from a map[string]any.
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
