package engine

import (
	"fmt"
	"strings"
)

func SliceToMap(list []string, del string) (map[string]string, error) {
	m := make(map[string]string)
	for _, e := range list {
		parts := strings.Split(e, del)
		if len(parts) == 2 {
			m[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		} else {
			return nil, fmt.Errorf("could not split '%s' into key value pair with '=' delimiter", e)
		}
	}

	return m, nil
}

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
