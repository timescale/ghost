package cmd

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/tutorial"
	"github.com/timescale/ghost/internal/util"
)

var (
	tutorialGenerateNameSuffix = generateTutorialNameSuffix

	tutorialTitleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Cyan)
	tutorialStepStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Cyan)
	tutorialRuleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	tutorialProseStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	tutorialLabelStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	tutorialCommandStyle = lipgloss.NewStyle().Foreground(lipgloss.Green)
	tutorialPromptStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	tutorialSuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Green)
)

func buildTutorialCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tutorial",
		Short: "Run an interactive Ghost tutorial",
		Long: `Run an interactive tutorial that demonstrates the core Ghost workflow.

The tutorial creates a temporary database, inserts sample data, forks the database,
mutates the fork, compares the original and fork, and then asks whether to delete
or keep the tutorial databases. Each step explains and echoes the equivalent Ghost
CLI command before running it.`,
		Example:           `  ghost tutorial`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTutorial(cmd, app)
		},
	}

	return cmd
}

func runTutorial(cmd *cobra.Command, app *common.App) (runErr error) {
	createdDatabaseNames := make([]string, 0, 2)
	defer func() {
		if runErr == nil || len(createdDatabaseNames) == 0 {
			return
		}

		cmd.PrintErrln()
		cmd.PrintErrln("Tutorial stopped before cleanup. To delete created databases later, run:")
		for i := len(createdDatabaseNames) - 1; i >= 0; i-- {
			cmd.PrintErrf("  ghost delete %s --confirm\n", createdDatabaseNames[i])
		}
	}()

	if !util.IsTerminal(cmd.InOrStdin()) {
		return errors.New("cannot run tutorial: stdin is not a terminal")
	}

	cfg, _, _, err := app.GetAll()
	if err != nil {
		return err
	}
	if cfg.ReadOnly {
		return errors.New("cannot run tutorial while read_only is enabled; run `ghost config set read_only false` to allow tutorial writes")
	}

	nameSuffix, err := tutorialGenerateNameSuffix()
	if err != nil {
		return err
	}

	originalDatabaseName := "tutorial-" + nameSuffix
	forkDatabaseName := originalDatabaseName + "-fork"
	promptReader := newTutorialPromptReader(cmd.InOrStdin())

	cmd.Println(tutorialTitleStyle.Render("Welcome to the Ghost tutorial!"))
	cmd.Println()
	cmd.Println(tutorialProseStyle.Render("This guided tour will run real Ghost commands to demonstrate the core workflow:"))
	cmd.Println(tutorialProseStyle.Render("create a database, load data, fork it, change the fork, compare the results, and clean up."))
	cmd.Println()
	cmd.Println(tutorialLabelStyle.Render("Temporary database names"))
	cmd.Printf("  original: %s\n", originalDatabaseName)
	cmd.Printf("  fork:     %s\n", forkDatabaseName)
	cmd.Println()

	t := tutorial.BuildLearnTheBasicsTutorial(originalDatabaseName, forkDatabaseName)
	for i, step := range t.Steps {
		if err := runTutorialStep(cmd, promptReader, i+1, step, &createdDatabaseNames); err != nil {
			return err
		}
	}

	deleteDatabases, err := promptTutorialCleanup(cmd, promptReader)
	if err != nil {
		return err
	}

	if !deleteDatabases {
		cmd.Println()
		cmd.Println(tutorialSuccessStyle.Render("Keeping the tutorial databases."))
		cmd.Println(tutorialProseStyle.Render("To clean them up later, run:"))
		cmd.Println(tutorialCommandStyle.Render("  ghost delete " + forkDatabaseName + " --confirm"))
		cmd.Println(tutorialCommandStyle.Render("  ghost delete " + originalDatabaseName + " --confirm"))
		return nil
	}

	cmd.Println()
	if err := runTutorialStep(cmd, promptReader, len(t.Steps)+1, t.DeleteStep, &createdDatabaseNames); err != nil {
		return err
	}

	cmd.Println(tutorialSuccessStyle.Render("Tutorial complete. You created, queried, forked, changed, compared, and deleted Ghost databases."))
	return nil
}

func runTutorialStep(cmd *cobra.Command, promptReader *tutorialPromptReader, number int, step tutorial.Step, createdDatabaseNames *[]string) error {
	printTutorialStep(cmd, number, step.Title)
	visibleBlocks := tutorial.FilterBlocks(step.Blocks, tutorial.TargetCLIOnly)
	for i, block := range visibleBlocks {
		if block.Prose != "" {
			cmd.Println(tutorialProseStyle.Render(block.Prose))
		}
		if len(block.Args) > 0 {
			if err := runTutorialCommand(cmd, promptReader, block.Args); err != nil {
				return err
			}
		}
		if block.CreatesDatabase != "" {
			*createdDatabaseNames = append(*createdDatabaseNames, block.CreatesDatabase)
		}
		if block.RemovesDatabase != "" {
			*createdDatabaseNames = removeTutorialName(*createdDatabaseNames, block.RemovesDatabase)
		}
		isLast := i == len(visibleBlocks)-1
		if !step.JoinedBlocks || isLast {
			cmd.Println()
		}
	}
	return nil
}

// runTutorialCommand displays the equivalent CLI invocation, waits for the
// user to press a key, then re-enters the root command tree to actually run
// it. The sub-execution writes directly to the user's real stdout/stderr so
// that output streams in real time and progress indicators (like the
// --wait spinner) work naturally.
func runTutorialCommand(cmd *cobra.Command, promptReader *tutorialPromptReader, args []string) error {
	printTutorialCommand(cmd, tutorial.FormatCommand(args))
	cmd.PrintErr(tutorialPromptStyle.Render("Press any key to run this command..."))
	if err := promptReader.readKey(cmd.Context()); err != nil {
		return fmt.Errorf("failed to read key: %w", err)
	}
	if util.IsTerminal(cmd.ErrOrStderr()) {
		// Erase the prompt line in place so it doesn't clutter scrollback,
		// leaving a blank line in its place for visual separation.
		cmd.PrintErr("\r\033[2K\n")
	} else {
		cmd.PrintErrln()
	}

	root := cmd.Root()
	// Forward any persistent flags the user set on the outer invocation
	// (e.g. --config-dir) so the sub-execution uses the same config and
	// state. --version-check=false is appended last so it overrides any
	// forwarded version-check value and prevents the update-available
	// banner from appearing once per step.
	root.SetArgs(append(tutorialForwardedFlags(root), append(args, "--version-check=false")...))
	if err := root.ExecuteContext(cmd.Context()); err != nil {
		// The sub-command's cobra dispatch already printed "Error: ..."
		// to stderr; silence the outer print so it doesn't appear twice.
		cmd.SilenceErrors = true
		return err
	}
	return nil
}

// tutorialForwardedFlags returns persistent flag args the user set on the
// outer invocation, so sub-executions see the same values. pflag.Visit only
// visits flags whose Changed field is true, so default values are not
// forwarded (they'll re-evaluate naturally during the sub-execution's flag
// parsing).
func tutorialForwardedFlags(root *cobra.Command) []string {
	var forwarded []string
	root.PersistentFlags().Visit(func(f *pflag.Flag) {
		forwarded = append(forwarded, fmt.Sprintf("--%s=%s", f.Name, f.Value.String()))
	})
	return forwarded
}

func printTutorialStep(cmd *cobra.Command, step int, title string) {
	heading := fmt.Sprintf("Step %d / %s", step, title)
	cmd.Println(tutorialStepStyle.Render(heading))
	cmd.Println(tutorialRuleStyle.Render(strings.Repeat("-", len(heading))))
}

func printTutorialCommand(cmd *cobra.Command, command string) {
	for i, line := range strings.Split(command, "\n") {
		prefix := "$ "
		if i > 0 {
			prefix = "  "
		}
		cmd.Println(tutorialCommandStyle.Render(prefix + line))
	}
}

func removeTutorialName(names []string, name string) []string {
	for i, n := range names {
		if n == name {
			return append(names[:i], names[i+1:]...)
		}
	}
	return names
}

func promptTutorialCleanup(cmd *cobra.Command, promptReader *tutorialPromptReader) (bool, error) {
	for {
		cmd.PrintErr("Delete the tutorial databases now? [Y/n] ")
		answer, err := promptReader.readLine(cmd.Context())
		if err != nil {
			return false, fmt.Errorf("failed to read cleanup choice: %w", err)
		}

		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "", "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			cmd.PrintErrln("Please answer y or n.")
		}
	}
}

func generateTutorialNameSuffix() (string, error) {
	bytes := make([]byte, 3)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate tutorial database name: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

type tutorialPromptReader struct {
	input         io.Reader
	bufferedInput *bufio.Reader
}

func newTutorialPromptReader(input io.Reader) *tutorialPromptReader {
	return &tutorialPromptReader{
		input:         input,
		bufferedInput: bufio.NewReader(input),
	}
}

func (r *tutorialPromptReader) readKey(ctx context.Context) error {
	if terminalInput, ok := r.input.(*os.File); ok && util.IsTerminal(r.input) {
		fd := int(terminalInput.Fd())
		state, err := term.MakeRaw(fd)
		if err != nil {
			return fmt.Errorf("failed to configure terminal: %w", err)
		}

		key, readErr := readTutorialValue(ctx, r.bufferedInput.ReadByte)
		restoreErr := term.Restore(fd, state)
		if readErr != nil {
			return readErr
		}
		if restoreErr != nil {
			return fmt.Errorf("failed to restore terminal: %w", restoreErr)
		}
		if key == byte(3) {
			return context.Canceled
		}
		return nil
	}

	_, err := r.readLine(ctx)
	return err
}

func (r *tutorialPromptReader) readLine(ctx context.Context) (string, error) {
	line, err := readTutorialValue(ctx, func() (string, error) {
		return r.bufferedInput.ReadString('\n')
	})
	return strings.TrimSpace(line), err
}

func readTutorialValue[T any](ctx context.Context, readFn func() (T, error)) (T, error) {
	type result struct {
		value T
		err   error
	}

	resultCh := make(chan result, 1)
	go func() {
		value, err := readFn()
		if ctx.Err() != nil {
			return
		}
		resultCh <- result{value: value, err: err}
	}()

	select {
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	case result := <-resultCh:
		return result.value, result.err
	}
}
