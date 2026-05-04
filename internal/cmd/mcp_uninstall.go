package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tailscale/hujson"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/mcp"
	"github.com/timescale/ghost/internal/util"
)

// uninstallTargetSelector is the function used to select an uninstall target
// interactively when no client argument is provided. It is a package-level
// variable so tests can override it without spinning up a real Bubble Tea
// program (which requires a TTY).
var uninstallTargetSelector = selectClientInteractively

func buildMCPUninstallCmd(_ *common.App) *cobra.Command {
	var noBackup bool
	var jsonOutput bool
	var yamlOutput bool

	cmd := &cobra.Command{
		Use:     "uninstall [client]",
		Aliases: []string{"remove", "rm"},
		Short:   "Uninstall Ghost MCP server configuration from a client",
		Long: `Uninstall the Ghost MCP server configuration from a supported MCP client.

Pass "all" to uninstall from all supported clients. If no client is specified, you'll be prompted to select one interactively.
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
		ValidArgs:    getValidMCPClientTargetNames(),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			targetName, err := selectedMCPUninstallTarget(cmd, args)
			if err != nil {
				return err
			}

			clients, err := mcpClientConfigsForTargetName(targetName)
			if err != nil {
				return err
			}

			results := uninstallGhostMCPFromClients(cmd.Context(), clients, !noBackup)
			output := make([]MCPClientStatusOutput, len(results))
			for i, result := range results {
				output[i] = MCPClientStatusOutput(result)
			}

			switch {
			case jsonOutput:
				err = util.SerializeToJSON(cmd.OutOrStdout(), output)
			case yamlOutput:
				err = util.SerializeToYAML(cmd.OutOrStdout(), output)
			default:
				err = outputMCPClientResultTable(cmd.OutOrStdout(), output)
			}
			if err != nil {
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
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&yamlOutput, "yaml", false, "Output in YAML format")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")

	return cmd
}

func selectedMCPUninstallTarget(cmd *cobra.Command, args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	if !util.IsTerminal(cmd.InOrStdin()) {
		return "", errors.New("no client specified and stdin is not a terminal; pass the client name or 'all' as an argument")
	}

	targetName, err := uninstallTargetSelector(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to select client: %w", err)
	}
	if targetName == "" {
		return "", errors.New("no client selected")
	}
	return targetName, nil
}

func uninstallGhostMCPFromClients(ctx context.Context, clients []clientConfig, createBackup bool) []MCPClientStatusOutput {
	results := make([]MCPClientStatusOutput, len(clients))
	for i, clientCfg := range clients {
		status, detail := uninstallGhostMCPFromClient(ctx, clientCfg, createBackup)
		results[i] = MCPClientStatusOutput{
			Client: clientCfg.ClientType,
			Status: status,
			Detail: detail,
		}
	}
	return results
}

func uninstallGhostMCPFromClient(ctx context.Context, clientCfg clientConfig, createBackup bool) (MCPClientStatus, string) {
	status, detail := detectMCPClientConfiguration(ctx, clientCfg)
	if status != mcpStatusConfigured {
		return status, detail
	}

	if clientCfg.buildUninstallCommand == nil {
		return uninstallGhostMCPFromJSONFiles(clientCfg, createBackup)
	}

	// uninstall via CLI command

	if createBackup {
		if backupErr := backupExistingConfigFiles(clientCfg.ConfigPaths); backupErr != nil {
			return mcpStatusError, backupErr.Error()
		}
	}

	args, err := clientCfg.buildUninstallCommand(mcp.ServerName)
	if err != nil {
		return mcpStatusError, fmt.Sprintf("failed to build uninstall command for %s: %v", clientCfg.Name, err)
	}
	output, err := runMCPClientCommand(ctx, args[0], args[1:]...)
	if err == nil {
		return mcpStatusUninstalled, ""
	}
	outputString := string(output)
	if errors.Is(err, exec.ErrNotFound) || strings.Contains(outputString, "No MCP server found") || strings.Contains(outputString, "No MCP servers are configured") || strings.Contains(outputString, "No MCP server named") {
		return mcpStatusNotConfigured, ""
	}
	return mcpStatusError, errorDetail(err, outputString)
}

func uninstallGhostMCPFromJSONFiles(clientCfg clientConfig, createBackup bool) (MCPClientStatus, string) {
	if clientCfg.MCPServersPathPrefix == "" {
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

		removed, err := removeGhostMCPFromJSONFile(expandedConfigPath, clientCfg.MCPServersPathPrefix, createBackup)
		if err != nil {
			return mcpStatusError, err.Error()
		}
		removedAny = removedAny || removed
	}

	if removedAny {
		return mcpStatusUninstalled, ""
	}
	return mcpStatusNotConfigured, ""
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

func mcpUninstallExitCode(results []MCPClientStatusOutput) int {
	anyUninstalled := false
	anyError := false
	for _, result := range results {
		switch result.Status {
		case mcpStatusUninstalled:
			anyUninstalled = true
		case mcpStatusError:
			anyError = true
		}
	}
	if anyError {
		return common.ExitGeneralError
	}
	if anyUninstalled {
		return 0
	}
	return mcpExitNoneConfigured
}
