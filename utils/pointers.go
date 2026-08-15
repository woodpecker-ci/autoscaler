package utils

// ToPtr returns a pointer to v.
func ToPtr[T any](v T) *T {
	return &v
}
