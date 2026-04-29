package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"
	"github.com/tailscale/hujson"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/mcp"
	"github.com/timescale/ghost/internal/util"
)

// MCPClientUninstallOutput represents a single client row in `ghost mcp uninstall` output.
type MCPClientUninstallOutput struct {
	Client string
	Status string
	Detail string
}

type mcpClientUninstallResult struct {
	Client string
	Status string
	Detail string
}

func buildMCPUninstallCmd(_ *common.App) *cobra.Command {
	var noBackup bool

	cmd := &cobra.Command{
		Use:   "uninstall [client]",
		Short: "Uninstall Ghost MCP server configuration from a client",
		Long: `Uninstall the Ghost MCP server configuration from a supported MCP client.

If no client is specified, you'll be prompted to select one interactively, including an "all" option.
Only the Ghost MCP server entry named "ghost" is removed; other MCP server entries are left untouched.`,
		Example: `  # Interactive client selection
  ghost mcp uninstall

  # Uninstall from Cursor
  ghost mcp uninstall cursor

  # Uninstall from all supported clients
  ghost mcp uninstall all

  # Skip backups when modifying config files
  ghost mcp uninstall cursor --no-backup`,
		Args:         cobra.MaximumNArgs(1),
		ValidArgs:    getValidUninstallTargetNames(),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			targetName, err := selectedMCPUninstallTarget(cmd, args)
			if err != nil {
				return err
			}

			clients, err := mcpUninstallTargetClients(targetName)
			if err != nil {
				return err
			}

			results := uninstallGhostMCPFromClients(cmd.Context(), clients, !noBackup)
			output := make([]MCPClientUninstallOutput, len(results))
			for i, result := range results {
				output[i] = MCPClientUninstallOutput(result)
			}

			if err := outputMCPClientUninstallResults(cmd.OutOrStdout(), output); err != nil {
				return err
			}

			exitCode := mcpUninstallExitCode(results)
			if exitCode == 0 {
				return nil
			}
			cmd.SilenceErrors = true
			return common.ExitWithCode(exitCode, nil)
		},
	}

	cmd.Flags().BoolVar(&noBackup, "no-backup", false, "Skip creating backup of existing configuration files (default: create backup)")

	return cmd
}

func getValidUninstallTargetNames() []string {
	validNames := getValidEditorNames()
	return append(validNames, mcpAllTarget)
}

func selectedMCPUninstallTarget(cmd *cobra.Command, args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	if !util.IsTerminal(cmd.InOrStdin()) {
		return "", errors.New("no client specified and stdin is not a terminal; pass the client name or 'all' as an argument")
	}

	targetName, err := selectUninstallTargetInteractively(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to select client: %w", err)
	}
	if targetName == "" {
		return "", errors.New("no client selected")
	}
	return targetName, nil
}

func mcpUninstallTargetClients(targetName string) ([]clientConfig, error) {
	if strings.EqualFold(targetName, mcpAllTarget) {
		clients := make([]clientConfig, len(supportedClients))
		copy(clients, supportedClients)
		return clients, nil
	}

	clientCfg, err := findClientConfig(targetName)
	if err != nil {
		return nil, err
	}
	return []clientConfig{*clientCfg}, nil
}

func uninstallGhostMCPFromClients(ctx context.Context, clients []clientConfig, createBackup bool) []mcpClientUninstallResult {
	results := make([]mcpClientUninstallResult, len(clients))
	for i, clientCfg := range clients {
		status, detail := uninstallGhostMCPFromClient(ctx, clientCfg, createBackup)
		results[i] = mcpClientUninstallResult{
			Client: clientCfg.Name,
			Status: status,
			Detail: detail,
		}
	}
	return results
}

func uninstallGhostMCPFromClient(ctx context.Context, clientCfg clientConfig, createBackup bool) (string, string) {
	status, detail := detectMCPClientConfiguration(ctx, clientCfg)
	if status != mcpStatusConfigured {
		return status, detail
	}

	switch clientCfg.ClientType {
	case ClaudeCode:
		return uninstallGhostMCPViaCLI(ctx, "claude", "mcp", "remove", "-s", "user", mcp.ServerName)
	case Codex:
		return uninstallGhostMCPViaCLI(ctx, "codex", "mcp", "remove", mcp.ServerName)
	case Gemini:
		return uninstallGhostMCPViaCLI(ctx, "gemini", "mcp", "remove", "-s", "user", mcp.ServerName)
	case KiroCLI:
		return uninstallGhostMCPViaCLI(ctx, "kiro-cli", "mcp", "remove", "--name", mcp.ServerName, "--scope", "global")
	case VSCode:
		return uninstallGhostMCPFromJSONFiles(clientCfg, vscodeMCPServersPathPrefix(), createBackup)
	default:
		return uninstallGhostMCPFromJSONFiles(clientCfg, clientCfg.MCPServersPathPrefix, createBackup)
	}
}

func uninstallGhostMCPViaCLI(ctx context.Context, command string, args ...string) (string, string) {
	output, err := runMCPClientCommand(ctx, command, args...)
	if err == nil {
		return "uninstalled", mcpStatusDetailMissing
	}
	outputString := string(output)
	if isExecutableNotFound(err) || strings.Contains(outputString, "No MCP server found") || strings.Contains(outputString, "No MCP servers are configured") || strings.Contains(outputString, "No MCP server named") {
		return mcpStatusUnconfigured, mcpStatusDetailMissing
	}
	return mcpStatusError, strings.TrimSpace(outputString)
}

func uninstallGhostMCPFromJSONFiles(clientCfg clientConfig, mcpServersPathPrefix string, createBackup bool) (string, string) {
	if mcpServersPathPrefix == "" {
		return mcpStatusError, fmt.Sprintf("missing MCP servers path for %s", clientCfg.Name)
	}

	removedAny := false
	for _, configPath := range clientCfg.ConfigPaths {
		expandedConfigPath := util.ExpandPath(configPath)
		if _, err := os.Stat(expandedConfigPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return mcpStatusError, fmt.Sprintf("failed to stat %s: %v", expandedConfigPath, err)
		}

		removed, err := removeGhostMCPFromJSONFile(expandedConfigPath, mcpServersPathPrefix, createBackup)
		if err != nil {
			return mcpStatusError, err.Error()
		}
		removedAny = removedAny || removed
	}

	if removedAny {
		return "uninstalled", mcpStatusDetailMissing
	}
	return mcpStatusUnconfigured, mcpStatusDetailMissing
}

func removeGhostMCPFromJSONFile(configPath, mcpServersPathPrefix string, createBackup bool) (bool, error) {
	serverConfig, exists, err := readMCPServerConfigFromJSONFile(configPath, mcpServersPathPrefix)
	if err != nil {
		return false, err
	}
	if !exists || !isExpectedGhostMCPCommand(serverConfig.Command, serverConfig.Args) {
		return false, nil
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		return false, fmt.Errorf("failed to read %s: %w", configPath, err)
	}
	if len(content) == 0 {
		content = []byte("{}")
	}

	value, err := hujson.Parse(content)
	if err != nil {
		return false, fmt.Errorf("failed to parse %s: %w", configPath, err)
	}

	if createBackup {
		if _, err := createConfigBackup(configPath); err != nil {
			return false, fmt.Errorf("failed to create backup for %s: %w", configPath, err)
		}
	}

	patchBytes, err := json.Marshal([]map[string]string{{
		"op":   "remove",
		"path": mcpServersPathPrefix + "/" + mcp.ServerName,
	}})
	if err != nil {
		return false, fmt.Errorf("failed to marshal remove patch: %w", err)
	}
	if err := value.Patch(patchBytes); err != nil {
		return false, fmt.Errorf("failed to remove Ghost MCP server from %s: %w", configPath, err)
	}

	formatted, err := hujson.Format(value.Pack())
	if err != nil {
		return false, fmt.Errorf("failed to format %s: %w", configPath, err)
	}

	fileMode := os.FileMode(0600)
	if info, err := os.Stat(configPath); err == nil {
		fileMode = info.Mode().Perm()
	}
	if err := os.WriteFile(configPath, formatted, fileMode); err != nil {
		return false, fmt.Errorf("failed to write %s: %w", configPath, err)
	}

	return true, nil
}

func mcpUninstallExitCode(results []mcpClientUninstallResult) int {
	anyUninstalled := false
	anyError := false
	for _, result := range results {
		switch result.Status {
		case "uninstalled":
			anyUninstalled = true
		case mcpStatusError:
			anyError = true
		}
	}
	if anyError {
		return mcpExitDetectionError
	}
	if anyUninstalled {
		return 0
	}
	return mcpExitNoneConfigured
}

func outputMCPClientUninstallResults(w io.Writer, results []MCPClientUninstallOutput) error {
	rows := make([]mcpClientResultRow, len(results))
	for i, result := range results {
		rows[i] = mcpClientResultRow(result)
	}
	return outputMCPClientResultTable(w, rows)
}

func selectUninstallTargetInteractively(cmd *cobra.Command) (string, error) {
	options := []ClientOption{{Name: "All supported clients", ClientName: mcpAllTarget}}
	for _, cfg := range supportedClients {
		options = append(options, ClientOption{
			Name:       cfg.Name,
			ClientName: cfg.EditorNames[0],
		})
	}

	model := clientSelectModel{options: options, cursor: 0}
	program := tea.NewProgram(model, tea.WithInput(cmd.InOrStdin()), tea.WithOutput(cmd.OutOrStdout()))
	finalModel, err := program.Run()
	if err != nil {
		return "", fmt.Errorf("failed to run client selection: %w", err)
	}

	result := finalModel.(clientSelectModel)
	if result.selected == "" {
		return "", errors.New("no client selected")
	}
	return result.selected, nil
}
