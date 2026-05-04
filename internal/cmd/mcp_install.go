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
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
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
func buildMCPInstallCmd(app *common.App) *cobra.Command {
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

Pass "all" to configure every supported client. If no client is specified, you'll be prompted to select one interactively.`, generateSupportedEditorsHelp()),
		Example: `  # Interactive client selection
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
			var clientName string
			if len(args) == 0 {
				if !util.IsTerminal(cmd.InOrStdin()) {
					return errors.New("no client specified and stdin is not a terminal; pass the client name or 'all' as an argument")
				}
				// No client specified, prompt user to select one
				var err error
				clientName, err = selectClientInteractively(cmd)
				if err != nil {
					return fmt.Errorf("failed to select client: %w", err)
				}
				if clientName == "" {
					return errors.New("no client selected")
				}
			} else {
				clientName = args[0]
			}

			if strings.EqualFold(clientName, mcpAllTarget) {
				return installGhostMCPForAllClients(cmd, !noBackup, jsonOutput, yamlOutput)
			}
			return installGhostMCPForClient(cmd, clientName, !noBackup, jsonOutput, yamlOutput)
		},
	}

	// Add flags
	cmd.Flags().BoolVar(&noBackup, "no-backup", false, "Skip creating backup of existing configuration (default: create backup)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&yamlOutput, "yaml", false, "Output in YAML format")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")

	return cmd
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
	// Optionally provide the check function for status detection (via CLI or other means)
	// If not provided, will default to JSON config detection
	detectInstallStatus func(ctx context.Context) (MCPClientStatus, string)
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
		detectInstallStatus: detectClaudeCodeMCPConfiguration,
	},
	{
		ClientType:           Cursor,
		Name:                 "Cursor",
		EditorNames:          []string{"cursor"},
		MCPServersPathPrefix: "/mcpServers",
		ConfigPaths: []string{
			"~/.cursor/mcp.json",
		},
	},
	{
		ClientType:           Windsurf,
		Name:                 "Windsurf",
		EditorNames:          []string{"windsurf"},
		MCPServersPathPrefix: "/mcpServers",
		ConfigPaths: []string{
			"~/.codeium/windsurf/mcp_config.json",
		},
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
		detectInstallStatus: detectCodexMCPConfiguration,
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
		detectInstallStatus: detectGeminiMCPConfiguration,
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
	},
	{
		ClientType:           Antigravity,
		Name:                 "Google Antigravity",
		EditorNames:          []string{"antigravity", "agy"},
		MCPServersPathPrefix: "/mcpServers",
		ConfigPaths: []string{
			"~/.gemini/antigravity/mcp_config.json",
		},
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
	},
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

// installGhostMCPForClient installs the Ghost MCP server configuration for the specified client.
// This is the Ghost-specific wrapper used by the CLI that handles defaults and success messages.
func installGhostMCPForClient(cmd *cobra.Command, clientName string, createBackup bool, jsonOutput, yamlOutput bool) error {
	clientCfg, err := findClientConfig(clientName)
	if err != nil {
		return err
	}

	statusOutput, err := installGhostMCPForClientWithoutOutput(cmd.Context(), *clientCfg, createBackup)
	if jsonOutput || yamlOutput {
		if outputErr := writeMCPInstallOutput(cmd, []MCPClientStatusOutput{statusOutput}, jsonOutput, yamlOutput); outputErr != nil {
			return outputErr
		}
		if err != nil {
			return err
		}
		return nil
	}
	if err != nil {
		return err
	}

	if statusOutput.Status == mcpStatusAlreadyConfigured {
		cmd.Printf("Ghost MCP server configuration for %s is already present\n", clientName)
		return nil
	}

	cmd.Printf("Successfully installed Ghost MCP server configuration for %s\n", clientName)
	cmd.Printf("Configuration file: %s\n", statusOutput.Detail)

	cmd.Printf("\nNext steps:\n")
	cmd.Printf("   1. Restart %s to load the new configuration\n", clientName)
	cmd.Printf("   2. The Ghost MCP server will be available as '%s'\n", mcp.ServerName)

	return nil
}

func installGhostMCPForAllClients(cmd *cobra.Command, createBackup bool, jsonOutput, yamlOutput bool) error {
	rows := make([]MCPClientStatusOutput, len(supportedClients))
	anyError := false
	for i, clientCfg := range supportedClients {
		row, err := installGhostMCPForClientWithoutOutput(cmd.Context(), clientCfg, createBackup)
		rows[i] = row
		if err != nil {
			anyError = true
		}
	}

	if err := writeMCPInstallOutput(cmd, rows, jsonOutput, yamlOutput); err != nil {
		return err
	}
	if anyError {
		cmd.SilenceErrors = true
		return common.ExitWithCode(common.ExitGeneralError, errors.New("failed to install Ghost MCP server for one or more clients"))
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
		return outputMCPClientResultTable(cmd.OutOrStdout(), rows)
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

func installGhostMCPForClientWithoutOutput(ctx context.Context, clientCfg clientConfig, createBackup bool) (MCPClientStatusOutput, error) {

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
		result.WriteString(fmt.Sprintf("  %-24s Configure for %s\n", primaryName, cfg.Name))
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

// ghostExecutablePathFunc can be overridden in tests to return a fixed path
var ghostExecutablePathFunc = defaultGetGhostExecutablePath

// getGhostExecutablePath returns the full path to the currently executing Ghost binary
func getGhostExecutablePath() (string, error) {
	return ghostExecutablePathFunc()
}

// defaultGetGhostExecutablePath is the default implementation
func defaultGetGhostExecutablePath() (string, error) {
	ghostPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	// If running via 'go run', os.Executable() returns a temp path like /tmp/go-build*/exe/ghost
	// In this case, return "ghost" assuming it's in PATH for development
	if strings.Contains(ghostPath, "go-build") && strings.Contains(ghostPath, "/exe/") {
		return "ghost", nil
	}

	return ghostPath, nil
}

// ClientOption represents a client choice for interactive selection
type ClientOption struct {
	Name       string // Display name
	ClientName string // Client name to pass to installMCPForClient
}

// selectClientInteractively prompts the user to select a client using Bubble Tea
func selectClientInteractively(cmd *cobra.Command) (string, error) {
	// Build client options from supportedClients
	clientOptions := make([]ClientOption, 0, len(supportedClients))
	for _, cfg := range supportedClients {
		// Use the first client name as the primary identifier
		primaryName := cfg.EditorNames[0]
		clientOptions = append(clientOptions, ClientOption{
			Name:       cfg.Name,
			ClientName: primaryName,
		})
	}

	// Sort options alphabetically by name, with "all" pinned at the top.
	sort.Slice(clientOptions, func(i, j int) bool {
		return clientOptions[i].Name < clientOptions[j].Name
	})
	options := append([]ClientOption{{Name: "All supported clients", ClientName: mcpAllTarget}}, clientOptions...)

	model := clientSelectModel{
		options: options,
		cursor:  0,
	}

	program := tea.NewProgram(model, tea.WithInput(cmd.InOrStdin()), tea.WithOutput(cmd.OutOrStdout()))
	finalModel, err := program.Run()
	if err != nil {
		return "", fmt.Errorf("failed to run editor selection: %w", err)
	}

	result := finalModel.(clientSelectModel)
	if result.selected == "" {
		return "", errors.New("no editor selected")
	}

	return result.selected, nil
}

// clientSelectModel represents the Bubble Tea model for client selection
type clientSelectModel struct {
	options      []ClientOption
	cursor       int
	selected     string
	numberBuffer string
}

func (m clientSelectModel) Init() tea.Cmd {
	return nil
}

func (m clientSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case "up", "k":
			// Clear buffer when using arrows
			m.numberBuffer = ""
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			// Clear buffer when using arrows
			m.numberBuffer = ""
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		case "enter", "space":
			m.selected = m.options[m.cursor].ClientName
			return m, tea.Quit
		case "backspace":
			// Handle backspace to remove last character from buffer
			if len(m.numberBuffer) > 0 {
				m.updateNumberBuffer(m.numberBuffer[:len(m.numberBuffer)-1])
			}
		case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
			// Add digit to buffer and update cursor position
			m.updateNumberBuffer(m.numberBuffer + msg.String())
		case "ctrl+w":
			// Clear buffer
			m.numberBuffer = ""
		}
	}
	return m, nil
}

// updateNumberBuffer moves the cursor to the editor matching the number buffer
func (m *clientSelectModel) updateNumberBuffer(newBuffer string) {
	if newBuffer == "" {
		m.numberBuffer = newBuffer
		return
	}

	// Parse the buffer as a number
	num, err := strconv.Atoi(newBuffer)
	if err != nil {
		return
	}

	// Convert from 1-based to 0-based index and validate bounds
	index := num - 1
	if index >= 0 && index < len(m.options) {
		m.numberBuffer = newBuffer
		m.cursor = index
	}
}

func (m clientSelectModel) View() tea.View {
	var s strings.Builder
	s.WriteString("Select an MCP client to configure:\n\n")

	for i, option := range m.options {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		s.WriteString(fmt.Sprintf("%s %d. %s\n", cursor, i+1, option.Name))
	}

	// Show the current number buffer if user is typing
	if m.numberBuffer != "" {
		s.WriteString(fmt.Sprintf("\nTyping: %s", m.numberBuffer))
	}

	s.WriteString("\nUse up/down arrows or number keys to navigate, enter to select, q to quit")
	return tea.NewView(s.String())
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
