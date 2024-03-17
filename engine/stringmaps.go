package engine

import (
	"fmt"
	"strings"
)

// SliceToMap converts a slice of strings in the format "key=value"
// into a string map, using the provided delimiter to split the pieces.
// Returns a map and nil error on success, or nil and an error if a
// slice element does not contain the delimiter.
func SliceToMap(list []string, del string) (map[string]string, error) {
	m := make(map[string]string)
	for _, e := range list {
		before, after, _ := strings.Cut(e, del)
		if before == "" || after == "" {
			return nil, fmt.Errorf("could not split '%s' into key value pair with '=' delimiter", e)
		}
		m[strings.TrimSpace(before)] = strings.TrimSpace(after)
	}

	return m, nil
}

// MergeMaps merges two string maps m1 and m2 into a new map.
// It copies all key-value pairs from m1 into the result.
// It then copies all key-value pairs from m2 into the result,
// overwriting any keys that are present in both m1 and m2.
// The merged map is returned.
func MergeMaps(m1, m2 map[string]string) map[string]string {
	merged := make(map[string]string)
	for k, v := range m1 {
		merged[k] = v
	}
	for key, value := range m2 {
		merged[key] = value
	}
	return merged
}
