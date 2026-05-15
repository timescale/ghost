package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/util"
)

// initStep identifies a top-level step of `ghost init`.
type initStep int

const (
	stepPATH initStep = iota
	stepLogin
	stepMCP
	stepCompletions
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
		Long:              `Interactively configure Ghost. Walks through adding Ghost to your PATH, login, MCP server installation, and shell completions.`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			err := runInit(cmd, app, skipIfConfigured)
			if err != nil && err.Error() == "" {
				// MCP install reports failures via its table and returns an
				// ExitCodeError with no message; suppress cobra's "Error: ..." line.
				cmd.SilenceErrors = true
			}
			return err
		},
	}

	cmd.Flags().BoolVar(&skipIfConfigured, "skip-if-configured", false, "Exit with a short message if every step is already configured")

	cmd.AddCommand(buildInitPathCmd())

	return cmd
}

func buildInitPathCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "path",
		Short:             "Add Ghost to your PATH",
		Long:              `Add Ghost's install directory to your PATH by appending a snippet to your shell rc file. This command does not prompt for confirmation, so it can be used from scripts.`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			changed, err := runInitPath(cmd)
			if err != nil {
				return err
			}
			if changed {
				cmd.PrintErrln("Restart your shell to apply changes.")
			}
			return nil
		},
	}
	return cmd
}

func runInit(cmd *cobra.Command, app *common.App, skipIfConfigured bool) error {
	ctx := cmd.Context()
	stdinIsTerminal := util.IsTerminal(cmd.InOrStdin())

	if !stdinIsTerminal && !skipIfConfigured {
		return errors.New("ghost init requires an interactive terminal; run it from a TTY")
	}

	states := detectInitStates(ctx, app)

	if skipIfConfigured && allConfigured(states) {
		cmd.PrintErrln("Ghost is already fully configured. Run `ghost init` to reconfigure.")
		return nil
	}

	if !stdinIsTerminal {
		return errors.New("ghost init requires an interactive terminal; run it from a TTY")
	}

	mainItems := buildMainMenuItems(states)
	result, err := common.RunMultiSelect(ctx, cmd.InOrStdin(), cmd.ErrOrStderr(), "Select what to configure:", mainItems)
	if err != nil {
		return err
	}
	switch result.Reason {
	case common.MultiSelectAborted, common.MultiSelectCanceled:
		cmd.PrintErrln("Canceled.")
		return nil
	}

	if len(result.Indices) == 0 {
		cmd.PrintErrln("Nothing selected.")
		return nil
	}

	if err := runSelectedInitSteps(cmd, app, result.Indices); err != nil {
		if errors.Is(err, common.ErrMultiSelectCanceled) || errors.Is(err, common.ErrMultiSelectAborted) {
			cmd.PrintErrln("Canceled.")
			return nil
		}
		return err
	}
	return nil
}

func runSelectedInitSteps(cmd *cobra.Command, app *common.App, indices []int) error {
	rcChanged := false
	for _, idx := range indices {
		switch initStep(idx) {
		case stepPATH:
			cmd.PrintErrln()
			cmd.PrintErrln("--- PATH ---")
			changed, err := runInitPath(cmd)
			if err != nil {
				return err
			}
			rcChanged = rcChanged || changed
		case stepLogin:
			if err := runInitLogin(cmd, app); err != nil {
				return err
			}
		case stepMCP:
			if err := runInitMCP(cmd); err != nil {
				return err
			}
		case stepCompletions:
			changed, err := runInitCompletions(cmd)
			if err != nil {
				return err
			}
			rcChanged = rcChanged || changed
		}
	}
	cmd.PrintErrln()
	if rcChanged {
		cmd.PrintErrln("All done. Restart your shell to apply changes.")
	} else {
		cmd.PrintErrln("All done.")
	}
	cmd.PrintErrln("\nGet started with:\n    ghost create\nFor help:\n    ghost --help")
	return nil
}

func detectInitStates(ctx context.Context, app *common.App) []initStepState {
	states := make([]initStepState, stepCount)
	states[stepPATH] = detectPathState()
	states[stepLogin] = detectLoginState(ctx, app)
	states[stepMCP] = detectMCPState(ctx)
	states[stepCompletions] = detectCompletionsState()
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
	if runtime.GOOS == "windows" {
		state.status = "unsupported on Windows — skipping"
		state.configured = true
		return state
	}
	shellType := common.DetectShellType()
	rc := common.DetectShellRC()
	if shellType == "" {
		state.status = "unsupported shell — skipping"
		state.configured = true
		return state
	}
	mentioned, err := common.ShellRCMentionsGhostCompletion(rc)
	if err != nil {
		state.status = fmt.Sprintf("could not read %s", rc)
		return state
	}
	if mentioned {
		state.configured = true
		state.status = fmt.Sprintf("already configured in %s", util.DisplayPath(rc))
		return state
	}
	state.status = fmt.Sprintf("not configured (would write to %s)", util.DisplayPath(rc))
	return state
}

// detectPathState reports whether the install dir is already in $PATH. On
// Windows it also consults the persistent user Path in the registry, since
// `ghost init` is typically run right after install and the current shell
// session's %PATH% hasn't been refreshed yet.
func detectPathState() initStepState {
	state := initStepState{label: "Add to PATH"}
	installDir, err := currentGhostInstallDir()
	if err != nil {
		state.status = "could not determine install location"
		return state
	}
	if installDir == "" {
		state.status = "not installed in a directory (e.g. run from source or via `npx ghost`)"
		state.configured = true
		return state
	}
	inPath := common.IsInPath(installDir)
	if !inPath && runtime.GOOS == "windows" {
		inUserPath, checkErr := common.IsInWindowsUserPath(installDir)
		if checkErr != nil {
			state.status = fmt.Sprintf("could not read user Path: %v", checkErr)
			return state
		}
		inPath = inUserPath
	}
	if inPath {
		state.configured = true
		state.status = fmt.Sprintf("already in PATH (%s)", util.DisplayPath(installDir))
		return state
	}
	state.status = fmt.Sprintf("not in PATH (%s)", util.DisplayPath(installDir))
	return state
}

func runInitLogin(cmd *cobra.Command, app *common.App) error {
	cmd.PrintErrln()
	cmd.PrintErrln("--- Login ---")
	result, err := common.Login(cmd.Context(), app, false, cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	cmd.PrintErrf("Logged in as %s\n", result.Email)
	return nil
}

func runInitMCP(cmd *cobra.Command) error {
	cmd.PrintErrln()
	cmd.PrintErrln("--- MCP server ---")

	clients, err := selectMCPClientsInteractively(cmd, mcpInstallSelectionOptions())
	if err != nil {
		return err
	}
	if len(clients) == 0 {
		cmd.PrintErrln("No MCP clients selected.")
		return nil
	}
	return installGhostMCPForClients(cmd, clients, true, false, false)
}

// runInitCompletions appends Ghost's completion snippet to the user's rc
// file. The returned bool reports whether the rc file was actually modified.
func runInitCompletions(cmd *cobra.Command) (bool, error) {
	cmd.PrintErrln()
	cmd.PrintErrln("--- Shell completions ---")
	if runtime.GOOS == "windows" {
		cmd.PrintErrln("Shell completions are not supported on Windows; skipping.")
		return false, nil
	}
	shellType := common.DetectShellType()
	if shellType == "" {
		cmd.PrintErrln("Could not detect your shell from $SHELL; skipping completions.")
		return false, nil
	}
	rc := common.DetectShellRC()
	mentioned, err := common.ShellRCMentionsGhostCompletion(rc)
	if err != nil {
		return false, fmt.Errorf("failed to read %s: %w", rc, err)
	}
	if mentioned {
		cmd.PrintErrf("Completions already configured in %s.\n", rc)
		return false, nil
	}

	binaryPath, err := getGhostExecutablePath()
	if err != nil {
		return false, fmt.Errorf("failed to determine Ghost executable path: %w", err)
	}
	if err := common.AppendCompletionsToShellRC(rc, shellType, binaryPath); err != nil {
		return false, err
	}
	cmd.PrintErrf("Added %s completions to %s.\n", shellType, rc)
	return true, nil
}

// runInitPath adds Ghost's install dir to the user's PATH. On Unix it
// appends a snippet to the shell rc file; on Windows it updates the user
// Path environment variable in the registry. The returned bool reports
// whether the change requires a shell restart to take effect.
func runInitPath(cmd *cobra.Command) (bool, error) {
	installDir, err := currentGhostInstallDir()
	if err != nil {
		return false, fmt.Errorf("failed to determine install directory: %w", err)
	}
	if common.IsInPath(installDir) {
		cmd.PrintErrf("%s is already in PATH.\n", installDir)
		return false, nil
	}
	if runtime.GOOS == "windows" {
		return runInitPathWindows(cmd, installDir)
	}
	rc := common.DetectShellRC()
	mentioned, err := common.ShellRCMentions(rc, installDir)
	if err != nil {
		return false, fmt.Errorf("failed to read %s: %w", rc, err)
	}
	if mentioned {
		cmd.PrintErrf("%s is already referenced in %s. Restart your shell to apply.\n", installDir, rc)
		return false, nil
	}
	if err := common.AppendPathToShellRC(rc, installDir); err != nil {
		return false, err
	}
	cmd.PrintErrf("Added %s to PATH in %s.\n", installDir, rc)
	return true, nil
}

// runInitPathWindows handles the PATH step on Windows by updating the user
// Path in the registry. It assumes the caller has already verified that
// installDir is not in the current session's %PATH%.
func runInitPathWindows(cmd *cobra.Command, installDir string) (bool, error) {
	inUserPath, err := common.IsInWindowsUserPath(installDir)
	if err != nil {
		return false, err
	}
	if inUserPath {
		cmd.PrintErrf("%s is already in your user Path. Restart your shell to apply.\n", installDir)
		return false, nil
	}
	if err := common.AddToWindowsUserPath(installDir); err != nil {
		return false, err
	}
	cmd.PrintErrf("Added %s to your user Path.\n", installDir)
	return true, nil
}

func currentGhostInstallDir() (string, error) {
	executablePath, err := getGhostExecutablePath()
	if err != nil {
		return "", err
	}
	if executablePath == "ghost" {
		return "", nil
	}
	return filepath.Dir(executablePath), nil
}
