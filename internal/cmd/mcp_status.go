package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tailscale/hujson"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/mcp"
	"github.com/timescale/ghost/internal/util"
)

const (
	mcpStatusConfigured    = "configured"
	mcpStatusUnconfigured  = "unconfigured"
	mcpStatusError         = "error"
	mcpExitNoneConfigured  = 1
	mcpExitDetectionError  = 2
	mcpStatusDetailMissing = ""
)

// MCPClientStatusOutput represents a single client row in `ghost mcp status` output.
type MCPClientStatusOutput struct {
	Client     string `json:"client"`
	ClientName string `json:"client_name"`
	Status     string `json:"status"`
	Detail     string `json:"detail,omitempty"`
}

type mcpClientStatusResult struct {
	Client     string
	ClientName string
	Status     string
	Detail     string
}

type mcpClientCommandRunner func(ctx context.Context, command string, args ...string) ([]byte, error)

var runMCPClientCommand = defaultRunMCPClientCommand

func defaultRunMCPClientCommand(ctx context.Context, command string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	return cmd.CombinedOutput()
}

func buildMCPStatusCmd(_ *common.App) *cobra.Command {
	var jsonOutput bool
	var yamlOutput bool

	cmd := &cobra.Command{
		Use:   "status [client]",
		Short: "Show Ghost MCP configuration status for supported clients",
		Long: `Show whether the Ghost MCP server is configured for supported MCP clients.

The command checks the selected client, or all supported clients when no client is specified.
A configured client must have a Ghost MCP server entry named "ghost" that runs "ghost mcp start".`,
		Example: `  # Check all supported clients
  ghost mcp status

  # Check Cursor only
  ghost mcp status cursor

  # Output as JSON
  ghost mcp status --json`,
		Args:         cobra.MaximumNArgs(1),
		ValidArgs:    getValidEditorNames(),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			clients, err := selectedMCPStatusClients(args)
			if err != nil {
				return err
			}

			results := detectMCPClientStatuses(cmd.Context(), clients)
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
				err = outputMCPClientStatuses(cmd.OutOrStdout(), output)
			}
			if err != nil {
				return err
			}

			exitCode := mcpStatusExitCode(results)
			if exitCode == 0 {
				return nil
			}
			cmd.SilenceErrors = true
			return common.ExitWithCode(exitCode, nil)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&yamlOutput, "yaml", false, "Output in YAML format")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")

	return cmd
}

func selectedMCPStatusClients(args []string) ([]clientConfig, error) {
	if len(args) == 0 {
		clients := make([]clientConfig, len(supportedClients))
		copy(clients, supportedClients)
		return clients, nil
	}

	clientCfg, err := findClientConfig(args[0])
	if err != nil {
		return nil, err
	}
	return []clientConfig{*clientCfg}, nil
}

func detectMCPClientStatuses(ctx context.Context, clients []clientConfig) []mcpClientStatusResult {
	results := make([]mcpClientStatusResult, len(clients))
	for i, clientCfg := range clients {
		result := detectMCPClientStatus(ctx, clientCfg)
		results[i] = result
	}
	return results
}

func detectMCPClientStatus(ctx context.Context, clientCfg clientConfig) mcpClientStatusResult {
	status, detail := detectMCPClientConfiguration(ctx, clientCfg)
	return mcpClientStatusResult{
		Client:     clientCfg.Name,
		ClientName: clientCfg.EditorNames[0],
		Status:     status,
		Detail:     detail,
	}
}

func detectMCPClientConfiguration(ctx context.Context, clientCfg clientConfig) (string, string) {
	switch clientCfg.ClientType {
	case ClaudeCode:
		return detectClaudeCodeMCPConfiguration(ctx)
	case Codex:
		return detectCodexMCPConfiguration(ctx)
	case Gemini:
		return detectGeminiMCPConfiguration(ctx)
	case KiroCLI:
		return detectKiroMCPConfiguration(ctx, clientCfg)
	case VSCode:
		return detectMCPConfigurationInJSONFiles(clientCfg, vscodeMCPServersPathPrefix())
	default:
		return detectMCPConfigurationInJSONFiles(clientCfg, clientCfg.MCPServersPathPrefix)
	}
}

func mcpStatusExitCode(results []mcpClientStatusResult) int {
	anyConfigured := false
	anyError := false
	for _, result := range results {
		switch result.Status {
		case mcpStatusConfigured:
			anyConfigured = true
		case mcpStatusError:
			anyError = true
		}
	}
	if anyError {
		return mcpExitDetectionError
	}
	if anyConfigured {
		return 0
	}
	return mcpExitNoneConfigured
}

func outputMCPClientStatuses(w io.Writer, statuses []MCPClientStatusOutput) error {
	rows := make([]mcpClientResultRow, len(statuses))
	for i, status := range statuses {
		rows[i] = mcpClientResultRow{
			Client: status.Client,
			Status: status.Status,
			Detail: status.Detail,
		}
	}
	return outputMCPClientResultTable(w, rows)
}

func detectClaudeCodeMCPConfiguration(ctx context.Context) (string, string) {
	output, err := runMCPClientCommand(ctx, "claude", "mcp", "get", mcp.ServerName)
	outputString := string(output)
	if err != nil {
		if isExecutableNotFound(err) || strings.Contains(outputString, "No MCP server found") || strings.Contains(outputString, "No MCP servers are configured") {
			return mcpStatusUnconfigured, mcpStatusDetailMissing
		}
		return mcpStatusError, strings.TrimSpace(outputString)
	}

	command := extractNamedValue(outputString, "Command")
	args := strings.Fields(extractNamedValue(outputString, "Args"))
	if isExpectedGhostMCPCommand(command, args) {
		return mcpStatusConfigured, mcpStatusDetailMissing
	}
	return mcpStatusUnconfigured, "ghost entry has unexpected command"
}

func detectCodexMCPConfiguration(ctx context.Context) (string, string) {
	output, err := runMCPClientCommand(ctx, "codex", "mcp", "list", "--json")
	if err != nil {
		if isExecutableNotFound(err) {
			return mcpStatusUnconfigured, mcpStatusDetailMissing
		}
		return mcpStatusError, strings.TrimSpace(string(output))
	}

	var servers []struct {
		Name      string `json:"name"`
		Transport struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"transport"`
	}
	if err := json.Unmarshal(output, &servers); err != nil {
		return mcpStatusError, fmt.Sprintf("failed to parse codex mcp list output: %v", err)
	}

	for _, server := range servers {
		if server.Name != mcp.ServerName {
			continue
		}
		if isExpectedGhostMCPCommand(server.Transport.Command, server.Transport.Args) {
			return mcpStatusConfigured, mcpStatusDetailMissing
		}
		return mcpStatusUnconfigured, "ghost entry has unexpected command"
	}
	return mcpStatusUnconfigured, mcpStatusDetailMissing
}

func detectGeminiMCPConfiguration(ctx context.Context) (string, string) {
	// `gemini mcp list` does not emit parseable output when stdout is not a TTY in the
	// tested version. The debug flag keeps the same list command but prints the server rows.
	output, err := runMCPClientCommand(ctx, "gemini", "mcp", "list", "--debug")
	outputString := string(output)
	if err != nil {
		if isExecutableNotFound(err) {
			return mcpStatusUnconfigured, mcpStatusDetailMissing
		}
		return mcpStatusError, strings.TrimSpace(outputString)
	}
	if strings.Contains(outputString, "No MCP servers configured") {
		return mcpStatusUnconfigured, mcpStatusDetailMissing
	}

	commandLine, ok := extractGeminiGhostCommandLine(outputString)
	if !ok {
		return mcpStatusUnconfigured, mcpStatusDetailMissing
	}
	fields := strings.Fields(commandLine)
	if len(fields) >= 1 && isExpectedGhostMCPCommand(fields[0], fields[1:]) {
		return mcpStatusConfigured, mcpStatusDetailMissing
	}
	return mcpStatusUnconfigured, "ghost entry has unexpected command"
}

func detectKiroMCPConfiguration(ctx context.Context, clientCfg clientConfig) (string, string) {
	output, err := runMCPClientCommand(ctx, "kiro-cli", "mcp", "status", "--name", mcp.ServerName)
	outputString := string(output)
	if err != nil {
		if isExecutableNotFound(err) || strings.Contains(outputString, "No MCP server named") {
			return mcpStatusUnconfigured, mcpStatusDetailMissing
		}
		return mcpStatusError, strings.TrimSpace(outputString)
	}

	command := extractNamedValue(outputString, "Command")
	if !isGhostExecutableCommand(command) {
		return mcpStatusUnconfigured, "ghost entry has unexpected command"
	}

	// Kiro's status output includes the command but not the args. Read Kiro's MCP
	// config file to verify the complete `ghost mcp start` command.
	fileStatus, detail := detectMCPConfigurationInJSONFiles(clientCfg, clientCfg.MCPServersPathPrefix)
	if fileStatus == mcpStatusConfigured {
		return mcpStatusConfigured, mcpStatusDetailMissing
	}
	if fileStatus == mcpStatusError {
		return fileStatus, detail
	}
	return mcpStatusUnconfigured, "ghost entry has unexpected command"
}

func detectMCPConfigurationInJSONFiles(clientCfg clientConfig, mcpServersPathPrefix string) (string, string) {
	if mcpServersPathPrefix == "" {
		return mcpStatusError, fmt.Sprintf("missing MCP servers path for %s", clientCfg.Name)
	}

	configured := false
	unexpectedCommand := false
	for _, configPath := range clientCfg.ConfigPaths {
		expandedConfigPath := util.ExpandPath(configPath)
		if _, err := os.Stat(expandedConfigPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return mcpStatusError, fmt.Sprintf("failed to stat %s: %v", expandedConfigPath, err)
		}

		serverConfig, exists, err := readMCPServerConfigFromJSONFile(expandedConfigPath, mcpServersPathPrefix)
		if err != nil {
			return mcpStatusError, err.Error()
		}
		if !exists {
			continue
		}
		if isExpectedGhostMCPCommand(serverConfig.Command, serverConfig.Args) {
			configured = true
		} else {
			unexpectedCommand = true
		}
	}

	if configured {
		return mcpStatusConfigured, mcpStatusDetailMissing
	}
	if unexpectedCommand {
		return mcpStatusUnconfigured, "ghost entry has unexpected command"
	}
	return mcpStatusUnconfigured, mcpStatusDetailMissing
}

func readMCPServerConfigFromJSONFile(configPath, mcpServersPathPrefix string) (MCPServerConfig, bool, error) {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return MCPServerConfig{}, false, fmt.Errorf("failed to read %s: %w", configPath, err)
	}
	if len(content) == 0 {
		content = []byte("{}")
	}

	value, err := hujson.Parse(content)
	if err != nil {
		return MCPServerConfig{}, false, fmt.Errorf("failed to parse %s: %w", configPath, err)
	}

	serverValue := value.Find(mcpServersPathPrefix + "/" + mcp.ServerName)
	if serverValue == nil {
		return MCPServerConfig{}, false, nil
	}

	var serverConfig MCPServerConfig
	if err := json.Unmarshal(serverValue.Pack(), &serverConfig); err != nil {
		return MCPServerConfig{}, false, fmt.Errorf("failed to parse %s Ghost MCP server config: %w", configPath, err)
	}
	return serverConfig, true, nil
}

func extractNamedValue(output, name string) string {
	pattern := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(name) + `\s*:\s*(.*?)\s*$`)
	match := pattern.FindStringSubmatch(output)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

func extractGeminiGhostCommandLine(output string) (string, bool) {
	pattern := regexp.MustCompile(`(?m)^.*\b` + regexp.QuoteMeta(mcp.ServerName) + `:\s*(.*?)\s*\(stdio\).*$`)
	match := pattern.FindStringSubmatch(output)
	if len(match) < 2 {
		return "", false
	}
	return match[1], true
}

func isExpectedGhostMCPCommand(command string, args []string) bool {
	return isGhostExecutableCommand(command) && len(args) == 2 && args[0] == "mcp" && args[1] == "start"
}

func isGhostExecutableCommand(command string) bool {
	base := strings.ToLower(filepath.Base(command))
	return base == "ghost" || base == "ghost.exe"
}

func isExecutableNotFound(err error) bool {
	var execErr *exec.Error
	return errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound)
}

func vscodeMCPServersPathPrefix() string {
	return "/servers"
}
