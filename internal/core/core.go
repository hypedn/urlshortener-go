package core

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"time"
)

type URL struct {
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	ShortCode string    `db:"short_code" json:"short_code"`
	LongURL   string    `db:"long_url" json:"long_url"`
}

// MaxURLLenght is the maximum allowed length used by Shorten operation.
const MaxURLLength = 2083

const (
	// base62Chars are the characters used for generating short codes.
	base62Chars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	// shortCodeLength is the length of the generated short codes.
	shortCodeLength = 6
)

// GenerateShortCode creates a random, URL-friendly string.
func GenerateShortCode() (string, error) {
	result := make([]byte, shortCodeLength)
	for i := range result {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(base62Chars))))
		if err != nil {
			return "", fmt.Errorf("generateShortCode: %w", err)
		}
		result[i] = base62Chars[num.Int64()]
	}
	return string(result), nil
}
