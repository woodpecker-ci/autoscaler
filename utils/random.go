package utils

import "math/rand/v2"

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

// RandomString returns a random string of length n. It uses the shared
// math/rand/v2 generator: seeding a fresh source per call with the wall clock
// produces identical strings for calls within one clock tick, which happens
// routinely on platforms with coarse clocks such as Windows.
func RandomString(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.IntN(len(letterRunes))]
	}
	return string(b)
}
