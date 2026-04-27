package common

import (
	"errors"
	"fmt"
	"net/url"
	"time"
)

// ShareURL returns the landing-page URL a recipient opens to consume a
// share. Uses url.JoinPath so callers don't have to care whether the
// configured base URL ends with a slash, and so the token is properly
// percent-encoded into the path. Returns an error if the configured base
// URL is malformed.
func ShareURL(baseURL, token string) (string, error) {
	joined, err := url.JoinPath(baseURL, token)
	if err != nil {
		return "", fmt.Errorf("invalid share_url %q: %w", baseURL, err)
	}
	return joined, nil
}

// ParseExpires parses an --expires / expires value into an absolute timestamp.
// Accepts either:
//   - a positive Go duration (e.g. "30m", "24h") — interpreted relative to now
//   - an RFC3339 timestamp (e.g. "2026-05-01T00:00:00Z")
//
// Returns nil for an empty input (no expiry). Returns an error for malformed
// input or non-positive durations.
func ParseExpires(value string, now time.Time) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	if d, err := time.ParseDuration(value); err == nil {
		if d <= 0 {
			return nil, errors.New("expires duration must be positive")
		}
		t := now.Add(d).UTC()
		return &t, nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return &t, nil
	}
	return nil, fmt.Errorf("invalid expires value %q (expected duration like 24h or RFC3339 timestamp like 2026-05-01T00:00:00Z)", value)
}
