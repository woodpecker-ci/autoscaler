package engine

import "strings"

func SliceToMap(list []string, del string) map[string]string {
	m := make(map[string]string)
	for _, e := range list {
		parts := strings.Split(e, del)
		m[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}

	return m
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
