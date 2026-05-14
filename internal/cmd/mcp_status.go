package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

type mcpClientCommandRunner func(ctx context.Context, command string, args ...string) ([]byte, error)

var runMCPClientCommand = func(ctx context.Context, command string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	return cmd.CombinedOutput()
}

func buildMCPStatusCmd(_ *common.App) *cobra.Command {
	var jsonOutput bool
	var yamlOutput bool

	cmd := &cobra.Command{
		Use:     "status [client]",
		Aliases: []string{"stat"},
		Short:   "Show Ghost MCP configuration status for supported clients",
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
		ValidArgs:    getValidMCPClientTargetNames(),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			target := mcpAllTarget
			if len(args) > 0 {
				target = args[0]
			}
			clients, err := mcpClientConfigsForTargetName(target)
			if err != nil {
				return err
			}

			results := detectMCPClientStatuses(cmd.Context(), clients)

			switch {
			case jsonOutput:
				err = util.SerializeToJSON(cmd.OutOrStdout(), results)
			case yamlOutput:
				err = util.SerializeToYAML(cmd.OutOrStdout(), results)
			default:
				err = outputMCPClientResultTable(cmd.OutOrStdout(), results)
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

func detectMCPClientStatuses(ctx context.Context, clients []clientConfig) []MCPClientStatusOutput {
	results := make([]MCPClientStatusOutput, len(clients))
	for i, clientCfg := range clients {
		result := detectMCPClientStatus(ctx, clientCfg)
		results[i] = result
	}
	return results
}

func detectMCPClientStatus(ctx context.Context, clientCfg clientConfig) MCPClientStatusOutput {
	status, detail := detectMCPClientConfiguration(ctx, clientCfg)
	return MCPClientStatusOutput{
		Client: clientCfg.ClientType,
		Status: status,
		Detail: detail,
	}
}

func detectMCPClientConfiguration(ctx context.Context, clientCfg clientConfig) (MCPClientStatus, string) {
	if clientCfg.detectInstallStatus != nil {
		return clientCfg.detectInstallStatus(ctx)
	}
	return detectMCPConfigurationInJSONFiles(clientCfg)
}

func mcpStatusExitCode(results []MCPClientStatusOutput) int {
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
		return common.ExitGeneralError
	}
	if anyConfigured {
		return 0
	}
	return mcpExitNoneConfigured
}

func detectClaudeCodeMCPConfiguration(ctx context.Context) (MCPClientStatus, string) {
	output, err := runMCPClientCommand(ctx, "claude", "mcp", "get", mcp.ServerName)
	outputString := string(output)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || strings.Contains(outputString, "No MCP server found") || strings.Contains(outputString, "No MCP servers are configured") {
			return mcpStatusNotConfigured, ""
		}
		return mcpStatusError, errorDetail(err, outputString)
	}

	command := extractNamedValue(outputString, "Command")
	args := strings.Fields(extractNamedValue(outputString, "Args"))
	if isExpectedGhostMCPCommand(command, args) {
		return mcpStatusConfigured, ""
	}
	return mcpStatusNotConfigured, "ghost entry has unexpected command"
}

func detectCodexMCPConfiguration(ctx context.Context) (MCPClientStatus, string) {
	output, err := runMCPClientCommand(ctx, "codex", "mcp", "list", "--json")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return mcpStatusNotConfigured, ""
		}
		return mcpStatusError, errorDetail(err, string(output))
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
			return mcpStatusConfigured, ""
		}
		return mcpStatusNotConfigured, "ghost entry has unexpected command"
	}
	return mcpStatusNotConfigured, ""
}

func detectGeminiMCPConfiguration(ctx context.Context) (MCPClientStatus, string) {
	// `gemini mcp list` does not emit parseable output when stdout is not a TTY in the
	// tested version. The debug flag keeps the same list command but prints the server rows.
	output, err := runMCPClientCommand(ctx, "gemini", "mcp", "list", "--debug")
	outputString := string(output)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return mcpStatusNotConfigured, ""
		}
		return mcpStatusError, errorDetail(err, outputString)
	}
	if strings.Contains(outputString, "No MCP servers configured") {
		return mcpStatusNotConfigured, ""
	}

	commandLine, ok := extractGeminiGhostCommandLine(outputString)
	if !ok {
		return mcpStatusNotConfigured, ""
	}
	fields := strings.Fields(commandLine)
	if len(fields) >= 1 && isExpectedGhostMCPCommand(fields[0], fields[1:]) {
		return mcpStatusConfigured, ""
	}
	return mcpStatusNotConfigured, "ghost entry has unexpected command"
}

func detectMCPConfigurationInJSONFiles(clientCfg clientConfig) (MCPClientStatus, string) {
	if clientCfg.MCPServersPathPrefix == "" {
		return mcpStatusError, fmt.Sprintf("missing MCP servers path for %s", clientCfg.Name)
	}

	unexpectedCommand := false
	for _, configPath := range clientCfg.ConfigPaths {
		expandedConfigPath := util.ExpandPath(configPath)
		serverConfig, exists, err := readMCPServerConfigFromJSONFile(expandedConfigPath, clientCfg.MCPServersPathPrefix)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return mcpStatusError, err.Error()
		}
		if !exists {
			continue
		}
		if isExpectedGhostMCPCommand(serverConfig.Command, serverConfig.Args) {
			return mcpStatusConfigured, ""
		} else {
			unexpectedCommand = true
		}
	}

	if unexpectedCommand {
		return mcpStatusNotConfigured, "ghost entry has unexpected command"
	}
	return mcpStatusNotConfigured, ""
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
	// Match a line of the form `  <name>: <value>` (e.g. `Command: ghost` or
	// `Args: mcp start`) anywhere in the output. Leading whitespace and
	// whitespace around the colon are tolerated; the value (group 1) is the
	// remainder of the line with surrounding whitespace trimmed.
	pattern := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(name) + `\s*:\s*(.*?)\s*$`)
	match := pattern.FindStringSubmatch(output)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

func extractGeminiGhostCommandLine(output string) (string, bool) {
	// Match a line from `gemini mcp list --debug` describing the ghost server,
	// which looks like `... ghost: <command and args> (stdio) ...`. The server
	// name is anchored on a word boundary so it does not match a substring of
	// another name, and group 1 captures everything between `ghost:` and the
	// trailing `(stdio)` marker, trimmed of surrounding whitespace.
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

// errorDetail returns a human-readable detail string for a failed external
// command invocation. It prefers the command's stdout/stderr output when
// non-empty, otherwise falls back to the underlying Go error so the user is
// never left with an empty detail column.
func errorDetail(err error, output string) string {
	detail := strings.TrimSpace(output)
	if err != nil && detail == "" {
		return err.Error()
	}
	return detail
}
