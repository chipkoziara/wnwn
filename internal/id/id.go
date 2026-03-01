// Package id provides ULID generation for task identifiers.
package id

import (
	"crypto/rand"
	"time"

	"github.com/oklog/ulid/v2"
)

// New generates a new ULID string using the current time.
func New() string {
	return ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String()
}

// NewAt generates a new ULID string using the specified time.
// Useful for testing or importing tasks with known creation times.
func NewAt(t time.Time) string {
	return ulid.MustNew(ulid.Timestamp(t), rand.Reader).String()
}

// Timestamp extracts the creation time encoded in a ULID string.
func Timestamp(id string) (time.Time, error) {
	parsed, err := ulid.Parse(id)
	if err != nil {
		return time.Time{}, err
	}
	return ulid.Time(parsed.Time()), nil
}
