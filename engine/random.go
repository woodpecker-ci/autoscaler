package engine

import (
	"math/rand"
	"time"
)

// RandomString generates a random string of length n using alphanumeric characters.
func RandomString(n int) string {
	letterRunes := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rng.Intn(len(letterRunes))]
	}
	return string(b)
}
