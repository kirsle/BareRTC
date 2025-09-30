package util

import (
	"math/rand"
	"regexp"
)

var reSafeString = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// RandomString returns a random string of any length.
func RandomString(n int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz"
	var result = make([]byte, n)
	for i := 0; i < n; i++ {
		result[i] = charset[rand.Intn(len(charset))]
	}
	return string(result)
}

// StripNasties sanitizes a string by removing all special characters, suitable for file names.
func StripNasties(v string) string {
	return reSafeString.ReplaceAllLiteralString(v, "")
}
