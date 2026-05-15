package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/spf13/cobra"
	"github.com/tailscale/hujson"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/mcp"
	"github.com/timescale/ghost/internal/util"
)

type MCPClientStatus string

const (
	// When installing, if successful and not previously configured
	mcpStatusInstalled MCPClientStatus = "installed"
	// When installing, if already present
	mcpStatusAlreadyConfigured MCPClientStatus = "already configured"
	// When checking status, if the client is configured correctly
	mcpStatusConfigured MCPClientStatus = "configured"
	// When checking status or uninstalling, if the client is not configured
	mcpStatusNotConfigured MCPClientStatus = "not configured"
	// When uninstalling, if successful
	mcpStatusUninstalled MCPClientStatus = "uninstalled"
	// Any error related to checking/updating the client
	mcpStatusError MCPClientStatus = "error"
)

type MCPClientStatusOutput struct {
	Client MCPClient       `json:"client"`
	Status MCPClientStatus `json:"status"`
	Detail string          `json:"detail,omitempty"`
}

const (
	mcpAllTarget          = "all"
	mcpExitNoneConfigured = 2
)

// buildMCPInstallCmd creates the install subcommand for configuring editors
func buildMCPInstallCmd(_ *common.App) *cobra.Command {
	var noBackup bool
	var jsonOutput bool
	var yamlOutput bool

	cmd := &cobra.Command{
		Use:     "install [client]",
		Aliases: []string{"add"},
		Short:   "Install and configure Ghost MCP server for a client",
		Long: fmt.Sprintf(`Install and configure the Ghost MCP server for a specific MCP client or AI assistant.

This command automates the configuration process by modifying the appropriate
configuration files for the specified client.

%s
The command will:
- Automatically detect the appropriate configuration file location
- Create the configuration directory if it doesn't exist
- Create a backup of existing configuration by default
- Merge with existing MCP server configurations (doesn't overwrite other servers)
- Validate the configuration after installation

Pass "all" to configure every supported client. If no client is specified, you'll be prompted to pick one or more clients interactively.`, generateSupportedEditorsHelp()),
		Example: `  # Interactive client selection (multi-select)
  ghost mcp install

  # Install for Claude Code (User scope - available in all projects)
  ghost mcp install claude-code

  # Install for Cursor IDE
  ghost mcp install cursor

  # Install for all supported clients
  ghost mcp install all

  # Install without creating backup
  ghost mcp install claude-code --no-backup`,
		Args:         cobra.MaximumNArgs(1),
		ValidArgs:    getValidMCPClientTargetNames(),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			clients, err := resolveMCPClients(cmd, args, mcpInstallSelectionOptions())
			if err != nil {
				if errors.Is(err, common.ErrMultiSelectCanceled) || errors.Is(err, common.ErrMultiSelectAborted) {
					cmd.PrintErrln("Canceled.")
					return nil
				}
				return err
			}
			if err := installGhostMCPForClients(cmd, clients, !noBackup, jsonOutput, yamlOutput); err != nil {
				// The per-row errors are already shown in the table, so suppress
				// cobra's "Error: ..." line.
				cmd.SilenceErrors = true
				return err
			}
			return nil
		},
	}

	// Add flags
	cmd.Flags().BoolVar(&noBackup, "no-backup", false, "Skip creating backup of existing configuration (default: create backup)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&yamlOutput, "yaml", false, "Output in YAML format")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")

	return cmd
}

func resolveMCPClients(cmd *cobra.Command, args []string, opts mcpClientSelectionOptions) ([]clientConfig, error) {
	if len(args) > 0 {
		return mcpClientConfigsForTargetName(args[0])
	}
	clients, err := selectMCPClientsInteractively(cmd, opts)
	if err != nil {
		return nil, err
	}
	if len(clients) == 0 {
		return nil, errors.New("no clients selected")
	}
	return clients, nil
}

// MCPClient represents our internal client types
type MCPClient string

const (
	ClaudeCode  MCPClient = "claude-code"
	Cursor      MCPClient = "cursor" // Both the IDE and the CLI
	Windsurf    MCPClient = "windsurf"
	Codex       MCPClient = "codex"
	Gemini      MCPClient = "gemini"
	VSCode      MCPClient = "vscode"
	Antigravity MCPClient = "antigravity"
	KiroCLI     MCPClient = "kiro-cli"
)

// MCPServerConfig represents the MCP server configuration
type MCPServerConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// clientConfig represents our own client configuration for Ghost MCP installation
type clientConfig struct {
	ClientType           MCPClient // Our internal client type
	Name                 string
	EditorNames          []string // Supported client names for this client
	MCPServersPathPrefix string   // JSON path prefix for MCP servers config (only for JSON config manipulation clients like Cursor/Windsurf)
	ConfigPaths          []string // Config file locations - used for backup on all clients, and for JSON manipulation on JSON-config clients
	// buildInstallCommand builds the CLI install command for CLI-based clients
	// Parameters: serverName (name to register), command (binary path), args (arguments to binary)
	buildInstallCommand   func(serverName, command string, args []string) ([]string, error)
	buildUninstallCommand func(serverName string) ([]string, error)
	// Optionally provide the check function for status detection (via CLI or other means).
	// If not provided, will default to JSON config detection.
	detectInstallStatus func(ctx context.Context) (MCPClientStatus, string)
	// Optionally provide best-effort client install detection. This is used only
	// to decide whether interactive install menus should preselect the client;
	// users can still manually select any supported client.
	detectClientInstalled func(ctx context.Context) bool
}

// supportedClients defines the clients we support for Ghost MCP installation
var supportedClients = []clientConfig{
	{
		ClientType:  ClaudeCode,
		Name:        "Claude Code",
		EditorNames: []string{"claude-code"},
		ConfigPaths: []string{
			"~/.claude.json",
		},
		buildInstallCommand: func(serverName, command string, args []string) ([]string, error) {
			return append([]string{"claude", "mcp", "add", "-s", "user", serverName, command}, args...), nil
		},
		buildUninstallCommand: func(serverName string) ([]string, error) {
			return []string{"claude", "mcp", "remove", "-s", "user", serverName}, nil
		},
		detectInstallStatus:   detectClaudeCodeMCPConfiguration,
		detectClientInstalled: detectClientExecutable("claude"),
	},
	{
		ClientType:  Codex,
		Name:        "Codex",
		EditorNames: []string{"codex"},
		ConfigPaths: []string{
			"~/.codex/config.toml",
			"$CODEX_HOME/config.toml",
		},
		buildInstallCommand: func(serverName, command string, args []string) ([]string, error) {
			return append([]string{"codex", "mcp", "add", serverName, command}, args...), nil
		},
		buildUninstallCommand: func(serverName string) ([]string, error) {
			return []string{"codex", "mcp", "remove", serverName}, nil
		},
		detectInstallStatus:   detectCodexMCPConfiguration,
		detectClientInstalled: detectClientExecutable("codex"),
	},
	{
		ClientType:           Cursor,
		Name:                 "Cursor",
		EditorNames:          []string{"cursor"},
		MCPServersPathPrefix: "/mcpServers",
		ConfigPaths: []string{
			"~/.cursor/mcp.json",
		},
		detectClientInstalled: detectClientExecutableOrPath([]string{"cursor"}, []string{
			"/Applications/Cursor.app",
			"~/Applications/Cursor.app",
			"/usr/share/applications/cursor.desktop",
			"~/.local/share/applications/cursor.desktop",
			"/opt/Cursor",
			"/opt/cursor",
		}),
	},
	{
		ClientType:  Gemini,
		Name:        "Gemini CLI",
		EditorNames: []string{"gemini", "gemini-cli"},
		ConfigPaths: []string{
			"~/.gemini/settings.json",
		},
		buildInstallCommand: func(serverName, command string, args []string) ([]string, error) {
			return append([]string{"gemini", "mcp", "add", "-s", "user", serverName, command}, args...), nil
		},
		buildUninstallCommand: func(serverName string) ([]string, error) {
			return []string{"gemini", "mcp", "remove", "-s", "user", serverName}, nil
		},
		detectInstallStatus:   detectGeminiMCPConfiguration,
		detectClientInstalled: detectClientExecutable("gemini"),
	},
	{
		ClientType:           Antigravity,
		Name:                 "Google Antigravity",
		EditorNames:          []string{"antigravity", "agy"},
		MCPServersPathPrefix: "/mcpServers",
		ConfigPaths: []string{
			"~/.gemini/antigravity/mcp_config.json",
		},
		detectClientInstalled: detectClientExecutableOrPath([]string{"antigravity", "agy"}, []string{
			"/Applications/Antigravity.app",
			"/Applications/Google Antigravity.app",
			"~/Applications/Antigravity.app",
			"~/Applications/Google Antigravity.app",
			"/usr/share/applications/antigravity.desktop",
			"~/.local/share/applications/antigravity.desktop",
		}),
	},
	{
		ClientType:           KiroCLI,
		Name:                 "Kiro CLI",
		EditorNames:          []string{"kiro-cli"},
		MCPServersPathPrefix: "/mcpServers",
		ConfigPaths: []string{
			"~/.kiro/settings/mcp.json",
		},
		buildInstallCommand: func(serverName, command string, args []string) ([]string, error) {
			return []string{"kiro-cli", "mcp", "add", "--name", serverName, "--scope", "global", "--force", "--command", command, "--args", strings.Join(args, ",")}, nil
		},
		buildUninstallCommand: func(serverName string) ([]string, error) {
			return []string{"kiro-cli", "mcp", "remove", "--name", serverName, "--scope", "global"}, nil
		},
		detectClientInstalled: detectClientExecutable("kiro-cli"),
	},
	{
		ClientType:  VSCode,
		Name:        "VS Code",
		EditorNames: []string{"vscode", "code", "vs-code"},
		ConfigPaths: []string{
			"~/.config/Code/User/mcp.json",
			"~/Library/Application Support/Code/User/mcp.json",
			"~/AppData/Roaming/Code/User/mcp.json",
		},
		MCPServersPathPrefix: "/servers",
		buildInstallCommand: func(serverName, command string, args []string) ([]string, error) {
			j, err := json.Marshal(map[string]any{
				"name":    serverName,
				"command": command,
				"args":    args,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to marshal MCP config: %w", err)
			}
			return []string{"code", "--add-mcp", string(j)}, nil
		},
		detectClientInstalled: detectClientExecutable("code"),
	},
	{
		ClientType:           Windsurf,
		Name:                 "Windsurf",
		EditorNames:          []string{"windsurf"},
		MCPServersPathPrefix: "/mcpServers",
		ConfigPaths: []string{
			"~/.codeium/windsurf/mcp_config.json",
		},
		detectClientInstalled: detectClientExecutableOrPath([]string{"windsurf"}, []string{
			"/Applications/Windsurf.app",
			"~/Applications/Windsurf.app",
			"/usr/share/applications/windsurf.desktop",
			"~/.local/share/applications/windsurf.desktop",
			"/opt/Windsurf",
			"/opt/windsurf",
		}),
	},
}

func detectClientExecutable(executableNames ...string) func(context.Context) bool {
	return detectClientExecutableOrPath(executableNames, nil)
}

func detectClientExecutableOrPath(executableNames []string, paths []string) func(context.Context) bool {
	return func(ctx context.Context) bool {
		if ctx.Err() != nil {
			return false
		}
		for _, executableName := range executableNames {
			if _, err := exec.LookPath(executableName); err == nil {
				return true
			}
		}
		for _, path := range paths {
			if _, err := os.Stat(util.ExpandPath(path)); err == nil {
				return true
			}
		}
		return false
	}
}

func detectMCPClientInstalled(ctx context.Context, clientCfg clientConfig) bool {
	if clientCfg.detectClientInstalled == nil {
		return true
	}
	return clientCfg.detectClientInstalled(ctx)
}

var supportedClientsMap = func() map[MCPClient]clientConfig {
	m := make(map[MCPClient]clientConfig)
	for _, client := range supportedClients {
		m[client.ClientType] = client
	}
	return m
}()

// getValidEditorNames returns all valid client names from supportedClients
func getValidEditorNames() []string {
	var validNames []string
	for _, client := range supportedClients {
		validNames = append(validNames, client.EditorNames...)
	}
	return validNames
}

func getValidMCPClientTargetNames() []string {
	validNames := getValidEditorNames()
	return append(validNames, mcpAllTarget)
}

func mcpClientConfigsForTargetName(targetName string) ([]clientConfig, error) {
	if strings.EqualFold(targetName, mcpAllTarget) {
		return supportedClients, nil
	}

	clientCfg, err := findClientConfig(targetName)
	if err != nil {
		return nil, err
	}
	return []clientConfig{*clientCfg}, nil
}

// installGhostMCPForClients installs Ghost MCP for the given client configs and
// renders the standard summary in the requested output format. A non-nil error
// is returned (after the table is written) when any single install fails.
func installGhostMCPForClients(cmd *cobra.Command, clients []clientConfig, createBackup bool, jsonOutput, yamlOutput bool) error {
	rows := make([]MCPClientStatusOutput, len(clients))
	anyError := false
	for i, clientCfg := range clients {
		row, err := installGhostMCPForClient(cmd.Context(), clientCfg, createBackup)
		rows[i] = row
		if err != nil {
			anyError = true
		}
	}

	if err := writeMCPInstallOutput(cmd, rows, jsonOutput, yamlOutput); err != nil {
		return err
	}
	if anyError {
		return common.ExitWithCode(common.ExitGeneralError, nil)
	}
	return nil
}

func writeMCPInstallOutput(cmd *cobra.Command, rows []MCPClientStatusOutput, jsonOutput, yamlOutput bool) error {
	switch {
	case jsonOutput:
		return util.SerializeToJSON(cmd.OutOrStdout(), rows)
	case yamlOutput:
		return util.SerializeToYAML(cmd.OutOrStdout(), rows)
	default:
		if err := outputMCPClientResultTable(cmd.OutOrStdout(), rows); err != nil {
			return err
		}
		if slices.ContainsFunc(rows, func(row MCPClientStatusOutput) bool { return row.Status == mcpStatusInstalled }) {
			cmd.Printf("\nNext steps:\n")
			what := "the client(s)"
			if len(rows) == 1 {
				what = supportedClientsMap[rows[0].Client].Name
			}
			cmd.Printf("   1. Restart %s to load the new configuration\n", what)
			cmd.Printf("   2. The Ghost MCP server will be available as '%s'\n", mcp.ServerName)
		}
		return nil
	}
}

func outputMCPClientResultTable(w io.Writer, rows []MCPClientStatusOutput) error {
	table := tablewriter.NewTable(w,
		tablewriter.WithHeaderAlignment(tw.AlignLeft),
		tablewriter.WithPadding(tw.Padding{Left: "", Right: "  ", Overwrite: true}),
		tablewriter.WithRendition(tw.Rendition{
			Borders: tw.Border{
				Left:   tw.Off,
				Right:  tw.Off,
				Top:    tw.Off,
				Bottom: tw.Off,
			},
			Settings: tw.Settings{
				Separators: tw.Separators{
					ShowHeader:     tw.Off,
					ShowFooter:     tw.Off,
					BetweenRows:    tw.Off,
					BetweenColumns: tw.Off,
				},
				Lines: tw.Lines{ShowHeaderLine: tw.Off},
			},
		}),
	)
	var hasDetail bool
	for _, row := range rows {
		if row.Detail != "" {
			hasDetail = true
			break
		}
	}
	if hasDetail {
		table.Header("CLIENT", "STATUS", "DETAIL")
	} else {
		table.Header("CLIENT", "STATUS")
	}
	for _, row := range rows {
		var name = supportedClientsMap[row.Client].Name
		if hasDetail {
			table.Append(name, row.Status, row.Detail)
		} else {
			table.Append(name, row.Status)
		}
	}
	return table.Render()
}

func installGhostMCPForClient(ctx context.Context, clientCfg clientConfig, createBackup bool) (MCPClientStatusOutput, error) {

	makeErrorResult := func(err error) (MCPClientStatusOutput, error) {
		return MCPClientStatusOutput{
			Client: clientCfg.ClientType,
			Status: mcpStatusError,
			Detail: err.Error(),
		}, err
	}

	status, detail := detectMCPClientConfiguration(ctx, clientCfg)
	if status == mcpStatusConfigured {
		return MCPClientStatusOutput{Client: clientCfg.ClientType, Status: mcpStatusAlreadyConfigured}, nil
	}
	if status == mcpStatusError {
		return MCPClientStatusOutput{Client: clientCfg.ClientType, Status: mcpStatusError, Detail: detail}, fmt.Errorf("failed to detect configuration: %s", detail)
	}

	command, err := getGhostExecutablePath()
	if err != nil {
		return makeErrorResult(fmt.Errorf("failed to get executable path: %w", err))
	}

	args := []string{"mcp", "start"}

	var configPath string
	if len(clientCfg.ConfigPaths) > 0 {
		// Use manual config path discovery for clients with configured paths
		configPath, err = findClientConfigFile(clientCfg)
		if err != nil {
			return makeErrorResult(fmt.Errorf("failed to find configuration for %s: %w", clientCfg.ClientType, err))
		}
	} else if clientCfg.buildInstallCommand == nil {
		// Client has neither ConfigPaths nor buildInstallCommand
		return makeErrorResult(fmt.Errorf("client %s has no ConfigPaths or buildInstallCommand defined", clientCfg.ClientType))
	}
	// else: CLI-only client - configPath remains empty, will use buildInstallCommand

	// Create backup if requested and we have a config file
	if createBackup && configPath != "" {
		_, err = createConfigBackup(configPath)
		if err != nil {
			return makeErrorResult(fmt.Errorf("failed to create backup: %w", err))
		}
	}

	// Add MCP server to configuration
	if clientCfg.buildInstallCommand != nil {
		// Use CLI approach when install command builder is configured
		if err := addMCPServerViaCLI(ctx, clientCfg, mcp.ServerName, command, args); err != nil {
			return makeErrorResult(fmt.Errorf("failed to add MCP server configuration via CLI: %w", err))
		}
	} else {
		// Use JSON patching approach for JSON-config clients
		if err := addMCPServerViaJSON(configPath, clientCfg.MCPServersPathPrefix, mcp.ServerName, command, args); err != nil {
			return makeErrorResult(fmt.Errorf("failed to add MCP server configuration via JSON: %w", err))
		}
	}

	return MCPClientStatusOutput{
		Client: clientCfg.ClientType,
		Status: mcpStatusInstalled,
		Detail: formatMCPInstallDetail(clientCfg.Name, configPath),
	}, nil
}

func formatMCPInstallDetail(clientName, configPath string) string {
	if configPath != "" {
		return configPath
	}
	return "managed by " + clientName
}

// findClientConfig finds the client configuration for a given client name
// This consolidates the logic of mapping client names to client types and finding the config
func findClientConfig(clientName string) (*clientConfig, error) {
	// Look up in our supported clients config
	for i := range supportedClients {
		for _, name := range supportedClients[i].EditorNames {
			if strings.EqualFold(name, clientName) {
				return &supportedClients[i], nil
			}
		}
	}

	// Build list of supported clients from our config for error message
	supportedNames := getValidEditorNames()

	return nil, fmt.Errorf("unsupported client: %s. Supported clients: %s", clientName, strings.Join(supportedNames, ", "))
}

// generateSupportedEditorsHelp generates the supported clients section for help text
func generateSupportedEditorsHelp() string {
	var result strings.Builder
	result.WriteString("Supported Clients:\n")
	for _, cfg := range supportedClients {
		// Show only the primary editor name in help text
		primaryName := cfg.EditorNames[0]
		fmt.Fprintf(&result, "  %-24s Configure for %s\n", primaryName, cfg.Name)
	}
	return result.String()
}

// findClientConfigFile finds a client configuration file from a list of possible paths
func findClientConfigFile(clientCfg clientConfig) (string, error) {
	if len(clientCfg.ConfigPaths) == 0 {
		return "", errors.New("no config paths provided")
	}

	for _, path := range clientCfg.ConfigPaths {
		// Expand environment variables and home directory
		expandedPath := util.ExpandPath(path)

		// Check if file exists
		if _, err := os.Stat(expandedPath); err == nil {
			return expandedPath, nil
		}
	}

	// If no existing config found, use the first path as default
	return util.ExpandPath(clientCfg.ConfigPaths[0]), nil
}

// path to the binary, but if we're running via 'go run' return "ghost" to allow detection in development without requiring a build
var getGhostExecutablePath = func() (string, error) {
	ghostPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	// If running via 'go run', os.Executable() returns a temp path like /tmp/go-build*/exe/ghost
	// In this case, return "ghost" assuming it's in PATH for development
	if (strings.Contains(ghostPath, "/go-build") && strings.Contains(ghostPath, "/exe/")) || strings.Contains(ghostPath, "/Caches/go-build") {
		return "ghost", nil
	}

	return ghostPath, nil
}

type mcpClientSelectionOptions struct {
	title             string
	statusText        func(MCPClientStatus, bool) string
	selectedByDefault func(MCPClientStatus, bool) bool
	dimmedByDefault   func(MCPClientStatus, bool) bool
	checkInstalled    bool
}

// mcpInstallSelectionOptions returns the multi-select options for picking
// MCP clients to install Ghost into. Shared by `ghost mcp install` and the
// MCP step of `ghost init`.
func mcpInstallSelectionOptions() mcpClientSelectionOptions {
	return mcpClientSelectionOptions{
		title:          "Select MCP clients to install:",
		checkInstalled: true,
		statusText: func(status MCPClientStatus, clientInstalled bool) string {
			switch status {
			case mcpStatusConfigured:
				return "already configured"
			case mcpStatusNotConfigured:
				if !clientInstalled {
					return "not configured (client not detected)"
				}
				return "not configured"
			case mcpStatusError:
				return "could not detect"
			default:
				return string(status)
			}
		},
		selectedByDefault: func(status MCPClientStatus, clientInstalled bool) bool {
			return status == mcpStatusNotConfigured && clientInstalled
		},
		dimmedByDefault: func(status MCPClientStatus, _ bool) bool {
			return status == mcpStatusConfigured
		},
	}
}

var selectMCPClientsInteractively = func(cmd *cobra.Command, options mcpClientSelectionOptions) ([]clientConfig, error) {
	if !util.IsTerminal(cmd.InOrStdin()) {
		return nil, errors.New("no client specified and stdin is not a terminal; pass the client name or 'all' as an argument")
	}

	items := make([]common.MultiSelectItem, len(supportedClients))
	for i, cfg := range supportedClients {
		status := detectMCPClientStatus(cmd.Context(), cfg)
		clientInstalled := true
		if options.checkInstalled {
			clientInstalled = detectMCPClientInstalled(cmd.Context(), cfg)
		}
		items[i] = common.MultiSelectItem{
			Label:    cfg.Name,
			Status:   options.statusText(status.Status, clientInstalled),
			Selected: options.selectedByDefault(status.Status, clientInstalled),
			Dimmed:   options.dimmedByDefault(status.Status, clientInstalled),
		}
	}

	result, err := common.RunMultiSelect(cmd.Context(), cmd.InOrStdin(), cmd.ErrOrStderr(), options.title, items)
	if err != nil {
		return nil, fmt.Errorf("failed to run client selection: %w", err)
	}
	switch result.Reason {
	case common.MultiSelectAborted:
		return nil, common.ErrMultiSelectAborted
	case common.MultiSelectCanceled:
		return nil, common.ErrMultiSelectCanceled
	}
	selected := make([]clientConfig, len(result.Indices))
	for i, idx := range result.Indices {
		selected[i] = supportedClients[idx]
	}
	return selected, nil
}

// addMCPServerViaCLI adds an MCP server using a CLI command configured in clientConfig.
// The provided context is forwarded to the subprocess so it can be cancelled by
// Ctrl+C / SIGINT / SIGTERM via the propagated command context.
func addMCPServerViaCLI(ctx context.Context, clientCfg clientConfig, serverName, command string, args []string) error {
	if clientCfg.buildInstallCommand == nil {
		return fmt.Errorf("no install command configured for client %s", clientCfg.Name)
	}

	// Build the install command with the provided parameters
	installCommand, err := clientCfg.buildInstallCommand(serverName, command, args)
	if err != nil {
		return fmt.Errorf("failed to build install command: %w", err)
	}

	// Run the configured CLI command
	cmd := exec.CommandContext(ctx, installCommand[0], installCommand[1:]...)

	// Capture output
	output, err := cmd.CombinedOutput()
	if err != nil {
		cmdStr := strings.Join(installCommand, " ")
		if string(output) != "" {
			return fmt.Errorf("failed to run %s installation command: %w\nCommand: %s\nOutput: %s", clientCfg.Name, err, cmdStr, string(output))
		}
		return fmt.Errorf("failed to run %s installation command: %w\nCommand: %s", clientCfg.Name, err, cmdStr)
	}

	return nil
}

// backupExistingConfigFiles backs up every existing file in configPaths.
// Missing files are skipped silently. The first error encountered is returned.
func backupExistingConfigFiles(configPaths []string) error {
	for _, configPath := range configPaths {
		expandedConfigPath := util.ExpandPath(configPath)
		if _, err := os.Stat(expandedConfigPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("failed to stat %s: %w", expandedConfigPath, err)
		}
		if _, err := createConfigBackup(expandedConfigPath); err != nil {
			return fmt.Errorf("failed to create backup for %s: %w", expandedConfigPath, err)
		}
	}
	return nil
}

// createConfigBackup creates a backup of the existing configuration file and returns the backup path
func createConfigBackup(configPath string) (string, error) {
	// Check if config file exists
	if _, err := os.Stat(configPath); errors.Is(err, fs.ErrNotExist) {
		// No existing config file, no backup needed
		return "", nil
	}

	backupPath := fmt.Sprintf("%s.backup.%d", configPath, time.Now().UnixNano())

	// Get original file mode, fallback to 0600 if unavailable
	origInfo, err := os.Stat(configPath)
	var mode fs.FileMode = 0600
	if err == nil {
		mode = origInfo.Mode().Perm()
	}

	// Read original file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read original config file: %w", err)
	}

	// Write backup file
	if err := os.WriteFile(backupPath, data, mode); err != nil {
		return "", fmt.Errorf("failed to write backup file: %w", err)
	}

	return backupPath, nil
}

// addMCPServerViaJSON adds an MCP server to the configuration file using JSON patching
func addMCPServerViaJSON(configPath, mcpServersPathPrefix, serverName, command string, args []string) error {
	// Create configuration directory if it doesn't exist
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create configuration directory %s: %w", configDir, err)
	}

	// MCP server configuration
	serverConfig := MCPServerConfig{
		Command: command,
		Args:    args,
	}

	// Get original file mode to preserve it, fallback to 0600 for new files
	var fileMode fs.FileMode = 0600
	if info, err := os.Stat(configPath); err == nil {
		fileMode = info.Mode().Perm()
	}

	// Read existing configuration or create empty one
	content, err := os.ReadFile(configPath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("failed to read config file: %w", err)
		}
		content = []byte("{}")
	}

	if len(content) == 0 {
		// If the file is empty, initialize with empty JSON object
		content = []byte("{}")
	}

	// Parse the JSON with hujson
	value, err := hujson.Parse(content)
	if err != nil {
		return fmt.Errorf("failed to parse existing config: %w", err)
	}

	// Check if the parent path exists using hujson's Find method
	// Find uses JSON Pointer format (RFC 6901) which matches our path format
	if value.Find(mcpServersPathPrefix) == nil {
		// Path doesn't exist, create it
		parentPatch := fmt.Sprintf(`[{ "op": "add", "path": "%s", "value": {} }]`, mcpServersPathPrefix)
		if err := value.Patch([]byte(parentPatch)); err != nil {
			return fmt.Errorf("failed to create MCP servers path: %w", err)
		}
	}

	// Marshal the MCP server data
	dataJSON, err := json.Marshal(serverConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal MCP server data: %w", err)
	}

	// Create JSON patch to add the MCP server
	patch := fmt.Sprintf(`[{ "op": "add", "path": "%s/%s", "value": %s }]`, mcpServersPathPrefix, serverName, dataJSON)

	// Apply the patch
	if err := value.Patch([]byte(patch)); err != nil {
		return fmt.Errorf("failed to apply JSON patch: %w", err)
	}

	// Format the result
	formatted, err := hujson.Format(value.Pack())
	if err != nil {
		return fmt.Errorf("failed to format patched JSON: %w", err)
	}

	// Write back to file (preserve original file mode)
	if err := os.WriteFile(configPath, formatted, fileMode); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
