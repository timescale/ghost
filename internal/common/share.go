package common

import (
	"fmt"
	"net/url"
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
