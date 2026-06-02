package common

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"time"

	"github.com/timescale/ghost/internal/api"
)

type FetchLogsArgs struct {
	Client      api.ClientWithResponsesInterface
	ProjectID   string
	DatabaseRef string
	Tail        int
	Until       time.Time
}

// leadingTimestampPattern matches log lines that already begin with an
// embedded timestamp in the form "YYYY-MM-DD HH:MM:SS" (with either a space
// or 'T' separator). Used to avoid double-prefixing a timestamp onto lines
// that already have one (e.g. pgbackrest output).
var leadingTimestampPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}`)

// formatLogEntry renders a structured log entry as a single string of the
// form `YYYY-MM-DD HH:MM:SS UTC <message>`. Lines that already begin with
// an embedded timestamp (e.g. pgbackrest output) are returned unprefixed.
func formatLogEntry(entry api.LogEntry) string {
	if leadingTimestampPattern.MatchString(entry.Message) {
		return entry.Message
	}
	return entry.Timestamp.UTC().Format("2006-01-02 15:04:05 UTC") + " " + entry.Message
}

// FetchLogs fetches database logs with cursor-based pagination up to the
// specified tail limit. Returns logs in ascending chronological order
// (oldest first), formatted as `YYYY-MM-DD HH:MM:SS UTC <message>`.
func FetchLogs(ctx context.Context, args FetchLogsArgs) ([]string, error) {
	params := &api.DatabaseLogsParams{
		Until: &args.Until,
	}

	// Set until to current time if not provided, so pagination is consistent
	// across multiple cursor requests.
	if params.Until.IsZero() {
		params.Until = new(time.Now())
	}

	var logs []string
	for {
		resp, err := args.Client.DatabaseLogsWithResponse(ctx, args.ProjectID, args.DatabaseRef, params)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch logs: %w", err)
		}

		if resp.StatusCode() != http.StatusOK {
			return nil, ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
		}

		if resp.JSON200 == nil {
			return nil, errors.New("empty response from API")
		}

		for _, entry := range resp.JSON200.Entries {
			logs = append(logs, formatLogEntry(entry))
			if len(logs) >= args.Tail {
				break
			}
		}

		if len(logs) >= args.Tail || resp.JSON200.LastCursor == nil {
			break
		}

		params.Cursor = resp.JSON200.LastCursor
	}

	// Reverse so oldest logs appear first (API returns newest first)
	slices.Reverse(logs)

	return logs, nil
}
