package analytics

import (
	"context"
	"maps"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/config"
)

// A list of properties that should never be recorded in analytics events.
// Used to filter flags and MCP tool call parameters from automatic tracking.
// Note that dashes in flag names are converted to underscores before being
// checked against this list.
var ignore = []string{
	"access_key",
	"secret_key",
	"project_id",
	"password",
	"new_password",
	"message",
	"share_token",
	"from_share",
}

type Analytics struct {
	config    *config.Config
	projectID string
	client    api.ClientWithResponsesInterface
}

// New initializes a new [Analytics] instance. The [config.Config] parameters
// is required, but the others are optional. Analytics won't be sent if the
// client is nil.
func New(cfg *config.Config, client api.ClientWithResponsesInterface, projectID string) *Analytics {
	return &Analytics{
		config:    cfg,
		projectID: projectID,
		client:    client,
	}
}

// Option is a function that modifies analytics event properties. Options are
// passed to Track and Identify methods to customize the data sent with events.
type Option func(properties map[string]any)

// Property creates an Option that adds a single key-value pair to the event
// properties. This is useful for adding custom analytics data that isn't
// covered by other Option functions.
func Property(key string, value any) Option {
	return func(properties map[string]any) {
		properties[key] = value
	}
}

// Map creates an Option that adds all key-value pairs from a map to the event
// properties. Keys specified in the ignore list are skipped.
//
// This is useful for including arbitrary map data (like MCP tool arguments) in
// analytics events without manually specifying each field.
func Map(m map[string]any) Option {
	return func(properties map[string]any) {
		for key, value := range m {
			if slices.Contains(ignore, key) {
				continue
			}
			properties[key] = value
		}
	}
}

// flagNameReplacer converts flag names from kebab-case to snake_case for
// consistent property naming in analytics events.
var flagNameReplacer = strings.NewReplacer("-", "_")

// FlagSet creates an Option that adds all flags that were explicitly set by
// the user (via Visit). Flag names are converted from kebab-case to snake_case
// (e.g., "no-wait" becomes "no_wait"). Flags in the ignore list are skipped.
//
// This is useful for tracking which flags users actually use when running commands.
func FlagSet(flagSet *pflag.FlagSet) Option {
	return func(properties map[string]any) {
		flagSet.Visit(func(flag *pflag.Flag) {
			key := flagNameReplacer.Replace(flag.Name)
			if slices.Contains(ignore, key) {
				return
			}
			properties[key] = flag.Value.String()
		})
	}
}

// Args creates an Option that adds command arguments to event properties,
// redacting sensitive positional arguments for specific commands (e.g. the
// password argument in "ghost password").
func Args(commandPath string, args []string) Option {
	return func(properties map[string]any) {
		filtered := slices.Clone(args)

		// Redact the password positional argument from "ghost password <name-or-id> [new-password]"
		if commandPath == "ghost password" && len(filtered) > 1 {
			filtered[1] = "[REDACTED]"
		}

		// Redact the feedback message from "ghost feedback [message]"
		if commandPath == "ghost feedback" && len(filtered) > 0 {
			filtered[0] = "[REDACTED]"
		}

		// Redact the share token from "ghost share revoke <share-token>"
		if commandPath == "ghost share revoke" && len(filtered) > 0 {
			filtered[0] = "[REDACTED]"
		}

		properties["args"] = filtered
	}
}

// Error creates an Option that adds success and error information to event
// properties. If err is nil, sets success: true. If err is not nil, sets
// success: false and includes the error message.
//
// This is commonly used at the end of command execution to track whether
// operations succeeded or failed, and what errors occurred.
func Error(err error) Option {
	return func(properties map[string]any) {
		if err != nil {
			properties["success"] = false
			properties["error"] = err.Error()
		} else {
			properties["success"] = true
		}
	}
}

// Identify associates the provided properties with the user for the sake of
// analytics. It automatically includes common properties like ProjectID. The
// identification is only sent if the client is initialized and analytics are
// enabled in the config, otherwise it is skipped.
func (a *Analytics) Identify(options ...Option) {
	// Create properties map with user-provided properties
	properties := map[string]any{}
	for _, option := range options {
		option(properties)
	}

	// Merge in default/common properties, overwriting user-provided properties
	// if there's a conflict (we always want these properties to be accurate)
	if a.projectID != "" {
		properties["project_id"] = a.projectID
	}

	// Check if analytics is disabled
	if !a.enabled() {
		return
	}

	// Check for cases where the client was not initialized
	// (e.g. because API credentials are not available)
	if a.client == nil {
		return
	}

	// Set a 5 second timeout for tracking analytics events. We intentionally
	// use context.Background() here so we can track events even if a command
	// times out or is canceled.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Send the event
	a.client.AnalyticsIdentifyWithResponse(ctx, api.AnalyticsIdentifyJSONRequestBody{
		Properties: properties,
	})
}

// Track sends an analytics event with the provided event name and properties.
// It automatically includes common properties like ProjectID, OS, and
// architecture. Events are only sent if the client is initialized and
// analytics are enabled in the config, otherwise they are skipped.
func (a *Analytics) Track(event string, options ...Option) {
	// Create properties map with user-provided properties
	properties := map[string]any{}
	for _, option := range options {
		option(properties)
	}

	// Merge in default/common properties, overwriting user-provided properties
	// if there's a conflict (we always want these properties to be accurate)
	maps.Copy(properties, map[string]any{
		"source":        "ghost",
		"ghost_version": config.Version,
		"os":            runtime.GOOS,
		"arch":          runtime.GOARCH,
	})
	if a.projectID != "" {
		properties["project_id"] = a.projectID
	}

	// Check if analytics is disabled
	if !a.enabled() {
		return
	}

	// Check for cases where the client was not initialized
	// (e.g. because API credentials are not available)
	if a.client == nil {
		return
	}

	// Set a 5 second timeout for tracking analytics events. We intentionally
	// use context.Background() here so we can track events even if a command
	// times out or is canceled.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Send the event
	a.client.AnalyticsTrackWithResponse(ctx, api.AnalyticsTrackJSONRequestBody{
		Event:      event,
		Properties: &properties,
	})
}

func (a *Analytics) enabled() bool {
	if envVarIsTrue("DO_NOT_TRACK") ||
		envVarIsTrue("NO_TELEMETRY") ||
		envVarIsTrue("DISABLE_TELEMETRY") {
		return false
	}

	return a.config.Analytics
}

func envVarIsTrue(envVar string) bool {
	b, _ := strconv.ParseBool(os.Getenv(envVar))
	return b
}
