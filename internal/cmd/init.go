package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/util"
)

// initStep identifies a top-level step of `ghost init`.
type initStep int

const (
	stepLogin initStep = iota
	stepMCP
	stepCompletions
	stepPATH
	stepCount
)

// initStepState carries the detected status for a single step.
type initStepState struct {
	label      string
	configured bool
	status     string
}

func buildInitCmd(app *common.App) *cobra.Command {
	var skipIfConfigured bool

	cmd := &cobra.Command{
		Use:               "init",
		Short:             "Interactively configure Ghost",
		Long:              `Interactively configure Ghost. Walks through login, MCP server installation, shell completions, and adding Ghost to your PATH.`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd, app, skipIfConfigured)
		},
	}

	cmd.Flags().BoolVar(&skipIfConfigured, "skip-if-configured", false, "Exit silently if every step is already configured")

	return cmd
}

func runInit(cmd *cobra.Command, app *common.App, skipIfConfigured bool) error {
	ctx := cmd.Context()

	states := detectInitStates(ctx, app)

	if skipIfConfigured && allConfigured(states) {
		cmd.PrintErrln("Ghost is already fully configured. Run `ghost init` to reconfigure.")
		return nil
	}

	if !util.IsTerminal(cmd.InOrStdin()) {
		return errors.New("ghost init requires an interactive terminal; run it from a TTY")
	}

	for {
		mainItems := buildMainMenuItems(states)
		result, err := common.RunMultiSelect(ctx, cmd.InOrStdin(), cmd.ErrOrStderr(), "Select what to configure:", mainItems)
		if err != nil {
			return err
		}
		switch result.Reason {
		case common.MultiSelectAborted:
			return common.ErrMultiSelectAborted
		case common.MultiSelectCanceled:
			cmd.PrintErrln("Canceled.")
			return nil
		}

		if len(result.Indices) == 0 {
			cmd.PrintErrln("Nothing selected.")
			return nil
		}

		retryMainMenu, err := runSelectedInitSteps(cmd, app, result.Indices)
		if err != nil {
			return err
		}
		if retryMainMenu {
			// User canceled out of a submenu. Re-detect state (login or
			// other steps may have changed) and show the main menu again.
			states = detectInitStates(ctx, app)
			continue
		}
		return nil
	}
}

// runSelectedInitSteps executes the steps the user picked. The returned bool
// reports whether the caller should redraw the main menu (true when a submenu
// was canceled, false on full success).
func runSelectedInitSteps(cmd *cobra.Command, app *common.App, indices []int) (bool, error) {
	for _, idx := range indices {
		switch initStep(idx) {
		case stepLogin:
			if err := runInitLogin(cmd, app); err != nil {
				return false, err
			}
		case stepMCP:
			retry, err := runInitMCP(cmd, app)
			if err != nil {
				return false, err
			}
			if retry {
				return true, nil
			}
		case stepCompletions:
			if err := runInitCompletions(cmd); err != nil {
				return false, err
			}
		case stepPATH:
			if err := runInitPath(cmd); err != nil {
				return false, err
			}
		}
	}
	cmd.PrintErrln()
	cmd.PrintErrln("All done. Restart your shell to pick up any rc file changes.")
	return false, nil
}

func detectInitStates(ctx context.Context, app *common.App) []initStepState {
	states := make([]initStepState, stepCount)
	states[stepLogin] = detectLoginState(ctx, app)
	states[stepMCP] = detectMCPState(ctx)
	states[stepCompletions] = detectCompletionsState()
	states[stepPATH] = detectPathState()
	return states
}

func allConfigured(states []initStepState) bool {
	return !slices.ContainsFunc(states, func(s initStepState) bool {
		return !s.configured
	})
}

func buildMainMenuItems(states []initStepState) []common.MultiSelectItem {
	items := make([]common.MultiSelectItem, len(states))
	for i, s := range states {
		items[i] = common.MultiSelectItem{
			Label:    s.label,
			Status:   s.status,
			Selected: !s.configured,
			Dimmed:   s.configured,
		}
	}
	return items
}

// detectLoginState validates that the stored credentials are still functional
// by calling /auth/info.
func detectLoginState(ctx context.Context, app *common.App) initStepState {
	state := initStepState{label: "Login to Ghost"}
	client, _, err := app.GetClient()
	if err != nil || client == nil {
		state.status = "not logged in"
		return state
	}
	resp, err := client.AuthInfoWithResponse(ctx)
	if err != nil || resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
		state.status = "credentials invalid (re-login required)"
		return state
	}
	email := ""
	if resp.JSON200.User != nil {
		email = resp.JSON200.User.Email
	} else if resp.JSON200.ApiKey != nil {
		email = resp.JSON200.ApiKey.UserEmail
	}
	if email != "" {
		state.status = "already configured (" + email + ")"
	} else {
		state.status = "already configured"
	}
	state.configured = true
	return state
}

// detectMCPState reports whether any supported MCP client is configured. The
// status shows up to three configured client names.
func detectMCPState(ctx context.Context) initStepState {
	state := initStepState{label: "Install MCP server"}
	var configuredNames []string
	for _, clientCfg := range supportedClients {
		result := detectMCPClientStatus(ctx, clientCfg)
		if result.Status == mcpStatusConfigured {
			configuredNames = append(configuredNames, clientCfg.Name)
		}
	}
	if len(configuredNames) == 0 {
		state.status = "no MCP clients configured"
		return state
	}
	state.configured = true
	if len(configuredNames) > 3 {
		state.status = fmt.Sprintf("already configured (%d clients)", len(configuredNames))
	} else {
		state.status = "already configured (" + strings.Join(configuredNames, ", ") + ")"
	}
	return state
}

// detectCompletionsState reports whether the shell rc already sources Ghost's
// completions.
func detectCompletionsState() initStepState {
	state := initStepState{label: "Shell completions"}
	shellType := common.DetectShellType()
	rc := common.DetectShellRC()
	if shellType == "" {
		state.status = "unsupported shell — skipping"
		state.configured = true
		return state
	}
	mentioned, err := common.ShellRCMentions(rc, "ghost completion")
	if err != nil {
		state.status = fmt.Sprintf("could not read %s", rc)
		return state
	}
	if mentioned {
		state.configured = true
		state.status = fmt.Sprintf("already configured in %s", displayPath(rc))
		return state
	}
	state.status = fmt.Sprintf("not configured (would write to %s)", displayPath(rc))
	return state
}

// detectPathState reports whether the install dir is already in $PATH.
func detectPathState() initStepState {
	state := initStepState{label: "Add to PATH"}
	exe, err := os.Executable()
	if err != nil {
		state.status = "could not determine install location"
		state.configured = true // best to skip rather than error
		return state
	}
	installDir := filepath.Dir(exe)
	if common.IsInPath(installDir) {
		state.configured = true
		state.status = fmt.Sprintf("already in PATH (%s)", displayPath(installDir))
		return state
	}
	state.status = fmt.Sprintf("not in PATH (%s)", displayPath(installDir))
	return state
}

func runInitLogin(cmd *cobra.Command, app *common.App) error {
	cmd.PrintErrln()
	cmd.PrintErrln("--- Login ---")
	result, err := common.Login(cmd.Context(), app, false, cmd.OutOrStdout())
	if err != nil {
		return err
	}
	cmd.PrintErrf("Logged in as %s\n", result.Email)
	return nil
}

func runInitMCP(cmd *cobra.Command, app *common.App) (bool, error) {
	cmd.PrintErrln()
	cmd.PrintErrln("--- MCP server ---")

	items := make([]common.MultiSelectItem, len(supportedClients))
	for i, clientCfg := range supportedClients {
		result := detectMCPClientStatus(cmd.Context(), clientCfg)
		configured := result.Status == mcpStatusConfigured
		var status string
		switch result.Status {
		case mcpStatusConfigured:
			status = "already configured"
		case mcpStatusNotConfigured:
			status = "not configured"
		case mcpStatusError:
			status = "could not detect"
		default:
			status = string(result.Status)
		}
		items[i] = common.MultiSelectItem{
			Label:    clientCfg.Name,
			Status:   status,
			Selected: !configured,
			Dimmed:   configured,
		}
	}

	res, err := common.RunMultiSelect(cmd.Context(), cmd.InOrStdin(), cmd.ErrOrStderr(), "Select MCP clients to install:", items)
	if err != nil {
		return false, err
	}
	switch res.Reason {
	case common.MultiSelectAborted:
		return false, common.ErrMultiSelectAborted
	case common.MultiSelectCanceled:
		return true, nil
	}

	if len(res.Indices) == 0 {
		cmd.PrintErrln("No MCP clients selected.")
		return false, nil
	}

	rows := make([]MCPClientStatusOutput, 0, len(res.Indices))
	anyError := false
	for _, idx := range res.Indices {
		row, err := installGhostMCPForClientWithoutOutput(cmd.Context(), supportedClients[idx], true)
		rows = append(rows, row)
		if err != nil {
			anyError = true
		}
	}

	if err := outputMCPClientResultTable(cmd.OutOrStdout(), rows); err != nil {
		return false, err
	}
	if anyError {
		cmd.PrintErrln("One or more clients failed to configure. See the table above.")
	}
	return false, nil
}

func runInitCompletions(cmd *cobra.Command) error {
	cmd.PrintErrln()
	cmd.PrintErrln("--- Shell completions ---")
	shellType := common.DetectShellType()
	if shellType == "" {
		cmd.PrintErrln("Could not detect your shell from $SHELL; skipping completions.")
		return nil
	}
	rc := common.DetectShellRC()
	mentioned, err := common.ShellRCMentions(rc, "ghost completion")
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", rc, err)
	}
	if mentioned {
		cmd.PrintErrf("Completions already configured in %s.\n", rc)
		return nil
	}

	binary := "ghost"
	if err := common.AppendCompletionsToShellRC(rc, shellType, binary); err != nil {
		return err
	}
	cmd.PrintErrf("Added %s completions to %s.\n", shellType, rc)
	cmd.PrintErrf("Restart your shell or run: source %s\n", rc)
	return nil
}

func runInitPath(cmd *cobra.Command) error {
	cmd.PrintErrln()
	cmd.PrintErrln("--- PATH ---")
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to determine install directory: %w", err)
	}
	installDir := filepath.Dir(exe)
	if common.IsInPath(installDir) {
		cmd.PrintErrf("%s is already in PATH.\n", installDir)
		return nil
	}
	rc := common.DetectShellRC()
	mentioned, err := common.ShellRCMentions(rc, installDir)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", rc, err)
	}
	if mentioned {
		cmd.PrintErrf("%s is already referenced in %s. Restart your shell to apply.\n", installDir, rc)
		return nil
	}
	if err := common.AppendPathToShellRC(rc, installDir); err != nil {
		return err
	}
	cmd.PrintErrf("Added %s to PATH in %s.\n", installDir, rc)
	cmd.PrintErrf("Restart your shell or run: source %s\n", rc)
	return nil
}

// displayPath replaces $HOME with ~ in path for compact display.
func displayPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(os.PathSeparator)) {
		return "~" + path[len(home):]
	}
	return path
}
