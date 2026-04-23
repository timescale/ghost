package common

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/timescale/ghost/internal/api"
)

// ShareURL returns the landing-page URL a recipient opens to consume a
// share. Uses url.JoinPath so callers don't have to care whether the
// configured base URL ends with a slash, and so the token is properly
// percent-encoded into the path.
func ShareURL(baseURL, token string) string {
	joined, err := url.JoinPath(baseURL, token)
	if err != nil {
		// Fall back to naïve join; a malformed base URL will surface
		// elsewhere (e.g. when the recipient opens the link) rather than
		// silently corrupting output here.
		return baseURL + "/" + token
	}
	return joined
}

// FindShareByToken looks up the API share matching the given token. The
// revoke API requires the internal share ID, so callers that want to let
// users revoke by token list shares and match client-side before calling
// RevokeShareWithResponse with the share's ID.
func FindShareByToken(ctx context.Context, client api.ClientWithResponsesInterface, projectID, token string) (api.DatabaseShare, error) {
	resp, err := client.ListSharesWithResponse(ctx, projectID)
	if err != nil {
		return api.DatabaseShare{}, fmt.Errorf("failed to list shares: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return api.DatabaseShare{}, ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
	}
	if resp.JSON200 == nil {
		return api.DatabaseShare{}, errors.New("empty response from API")
	}
	for _, s := range *resp.JSON200 {
		if s.ShareToken == token {
			return s, nil
		}
	}
	return api.DatabaseShare{}, errors.New("share not found for the given token")
}
