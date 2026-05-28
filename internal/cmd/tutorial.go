package cmd

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/tutorial"
	"github.com/timescale/ghost/internal/util"
)

var (
	tutorialGenerateNameSuffix = generateTutorialNameSuffix

	tutorialTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Cyan)
	tutorialStepStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Cyan)
	tutorialRuleStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	tutorialProseStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	tutorialLabelStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	tutorialCommandStyle  = lipgloss.NewStyle().Foreground(lipgloss.Green)
	tutorialPromptStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	tutorialSuccessStyle  = lipgloss.NewStyle().Foreground(lipgloss.Green)
	tutorialCanceledStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
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

func runTutorial(cmd *cobra.Command, app *common.App) error {
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

	// Wrap stdin in a bufio.Reader so repeated util.ReadLine calls share
	// buffered state. util.ReadLine creates a fresh bufio.NewReader per
	// call, but bufio.NewReader returns its input unchanged when that input
	// is already a *bufio.Reader, so wrapping once here turns multi-prompt
	// reads into a single ongoing buffer — important for tests that feed
	// every prompt from one strings.Reader. The wrap is harmless in
	// production because cooked-mode stdin only delivers one line at a
	// time anyway.
	cmd.SetIn(bufio.NewReader(cmd.InOrStdin()))

	originalDatabaseName := "tutorial-" + nameSuffix
	forkDatabaseName := originalDatabaseName + "-fork"
	createdDatabaseNames := make([]string, 0, 2)

	flowErr := runTutorialFlow(cmd, originalDatabaseName, forkDatabaseName, &createdDatabaseNames)

	if errors.Is(flowErr, context.Canceled) {
		handleTutorialCancellation(cmd, createdDatabaseNames)
		return nil
	}
	return flowErr
}

func runTutorialFlow(cmd *cobra.Command, originalDatabaseName, forkDatabaseName string, createdDatabaseNames *[]string) error {
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
		if err := runTutorialStep(cmd, i+1, step, createdDatabaseNames); err != nil {
			return err
		}
	}

	deleteDatabases, err := promptTutorialCleanup(cmd)
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
	if err := runTutorialStep(cmd, len(t.Steps)+1, t.DeleteStep, createdDatabaseNames); err != nil {
		return err
	}

	cmd.Println(tutorialSuccessStyle.Render("Tutorial complete. You created, queried, forked, changed, compared, and deleted Ghost databases."))
	return nil
}

func runTutorialStep(cmd *cobra.Command, number int, step tutorial.Step, createdDatabaseNames *[]string) error {
	printTutorialStep(cmd, number, step.Title)
	visibleBlocks := tutorial.FilterBlocks(step.Blocks, tutorial.TargetCLIOnly)
	for i, block := range visibleBlocks {
		if block.Prose != "" {
			cmd.Println(tutorialProseStyle.Render(block.Prose))
		}
		if len(block.Args) > 0 {
			if err := runTutorialBlock(cmd, block, createdDatabaseNames); err != nil {
				return err
			}
		}
		isLast := i == len(visibleBlocks)-1
		if !step.JoinedBlocks || isLast {
			cmd.Println()
		}
	}
	return nil
}

// runTutorialBlock prompts the user with "Press any key...", then executes
// the block's sub-command and updates the tracked database list. The
// CreatesDatabase name is appended *before* the sub-command runs so that a
// partial failure (e.g. Ctrl+C during --wait after the API already created
// the database) still leaves the name available to the cleanup flow.
// RemovesDatabase is applied only after a successful run.
func runTutorialBlock(cmd *cobra.Command, block tutorial.Block, createdDatabaseNames *[]string) error {
	printTutorialCommand(cmd, tutorial.FormatCommand(block.Args))
	if err := promptPressEnter(cmd); err != nil {
		return err
	}
	if block.CreatesDatabase != "" {
		*createdDatabaseNames = append(*createdDatabaseNames, block.CreatesDatabase)
	}
	if err := executeTutorialSubCommand(cmd, block.Args); err != nil {
		return err
	}
	if block.RemovesDatabase != "" {
		*createdDatabaseNames = slices.DeleteFunc(*createdDatabaseNames, func(name string) bool {
			return name == block.RemovesDatabase
		})
	}
	return nil
}

// promptPressEnter prompts and waits for the user to press Enter. The
// read goes through util.ReadLine on stdin in its default cooked mode,
// so a terminal Ctrl+C generates SIGINT — main.go's signal handler
// cancels ctx, which the ReadLine select picks up and surfaces as
// context.Canceled.
func promptPressEnter(cmd *cobra.Command) error {
	cmd.PrintErr(tutorialPromptStyle.Render("Press Enter to run this command..."))
	_, err := util.ReadLine(cmd.Context(), cmd.InOrStdin())
	cmd.PrintErrln()
	return err
}

// executeTutorialSubCommand dispatches back into the root command tree to
// run the given sub-command with the user's persistent flags.
// root.SilenceErrors is set during the sub-execution so cobra doesn't
// print the inner error itself; the outer tutorial layer either turns a
// cancellation into the graceful cleanup flow (no error printed) or lets
// the outer cobra Execute print real errors once.
func executeTutorialSubCommand(cmd *cobra.Command, args []string) error {
	root := cmd.Root()
	previousSilenceErrors := root.SilenceErrors
	root.SilenceErrors = true
	defer func() { root.SilenceErrors = previousSilenceErrors }()

	root.SetArgs(append(tutorialForwardedFlags(root), append(args, "--version-check=false")...))
	return root.ExecuteContext(cmd.Context())
}

// tutorialForwardedFlags returns persistent flag args the user set on the
// outer invocation so sub-executions see the same values. pflag.Visit only
// visits flags whose Changed field is true, so default values are not
// forwarded (they'll re-evaluate naturally during the sub-execution's flag
// parsing). The "--version-check=false" arg is appended later by the
// caller to suppress per-step update banners.
func tutorialForwardedFlags(root *cobra.Command) []string {
	var forwarded []string
	root.PersistentFlags().Visit(func(f *pflag.Flag) {
		forwarded = append(forwarded, fmt.Sprintf("--%s=%s", f.Name, f.Value.String()))
	})
	return forwarded
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

func printTutorialStep(cmd *cobra.Command, step int, title string) {
	heading := fmt.Sprintf("Step %d / %s", step, title)
	cmd.Println(tutorialStepStyle.Render(heading))
	cmd.Println(tutorialRuleStyle.Render(strings.Repeat("-", len(heading))))
}

// promptTutorialCleanup asks at the end of the happy-path flow whether to
// delete the databases that the tutorial created. Ctrl+C here is treated
// as "no" — the user has already finished the meaningful part of the
// tutorial, so we just print the manual cleanup instructions rather than
// kicking off the cancellation flow's redundant second prompt.
func promptTutorialCleanup(cmd *cobra.Command) (bool, error) {
	for {
		cmd.PrintErr("Delete the tutorial databases now? [Y/n] ")
		answer, err := util.ReadLine(cmd.Context(), cmd.InOrStdin())
		if errors.Is(err, context.Canceled) {
			return false, nil
		}
		if err != nil {
			return false, err
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

// handleTutorialCancellation runs the post-Ctrl+C cleanup flow: prints a
// brief "canceled" message and, if any databases were created, asks
// whether to delete them. Uses cmd.Context() directly — Ctrl+C during the
// spinner doesn't generate SIGINT (the spinner's own model intercepts it),
// so the parent context is still alive here.
func handleTutorialCancellation(cmd *cobra.Command, createdDatabaseNames []string) {
	cmd.PrintErrln()
	cmd.PrintErrln(tutorialCanceledStyle.Render("Tutorial canceled."))

	if len(createdDatabaseNames) == 0 {
		return
	}

	cmd.PrintErrln()
	if len(createdDatabaseNames) == 1 {
		cmd.PrintErrln("1 tutorial database was created:")
	} else {
		cmd.PrintErrf("%d tutorial databases were created:\n", len(createdDatabaseNames))
	}
	for _, name := range createdDatabaseNames {
		cmd.PrintErrf("  %s\n", name)
	}
	cmd.PrintErrln()

	confirm, err := promptCleanupAfterCancel(cmd, len(createdDatabaseNames))
	if err != nil || !confirm {
		printManualCleanupHint(cmd, createdDatabaseNames)
		return
	}

	if err := runCleanupDeletes(cmd, createdDatabaseNames); err != nil {
		cmd.PrintErrln()
		cmd.PrintErrln(tutorialCanceledStyle.Render("Cleanup did not complete."))
		printManualCleanupHint(cmd, createdDatabaseNames)
	}
}

func promptCleanupAfterCancel(cmd *cobra.Command, count int) (bool, error) {
	for {
		if count == 1 {
			cmd.PrintErr("Delete it now? [Y/n] ")
		} else {
			cmd.PrintErr("Delete them now? [Y/n] ")
		}
		answer, err := util.ReadLine(cmd.Context(), cmd.InOrStdin())
		if err != nil {
			return false, err
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

func printManualCleanupHint(cmd *cobra.Command, createdDatabaseNames []string) {
	cmd.PrintErrln()
	cmd.PrintErrln(tutorialProseStyle.Render("To delete them later, run:"))
	// Reverse order so forks are deleted before their parent originals.
	for i := len(createdDatabaseNames) - 1; i >= 0; i-- {
		cmd.PrintErrln(tutorialCommandStyle.Render("  ghost delete " + createdDatabaseNames[i] + " --confirm"))
	}
}

// runCleanupDeletes invokes "ghost delete <name> --confirm" for each
// created database, in reverse order (forks first). Unlike the happy-path
// Step 7 deletes, there's no "press any key" prompt — the user has
// already opted in to cleanup via the cancellation prompt.
func runCleanupDeletes(cmd *cobra.Command, createdDatabaseNames []string) error {
	for i := len(createdDatabaseNames) - 1; i >= 0; i-- {
		args := []string{"delete", createdDatabaseNames[i], "--confirm"}
		cmd.PrintErrln()
		printTutorialCommand(cmd, tutorial.FormatCommand(args))
		if err := executeTutorialSubCommand(cmd, args); err != nil {
			return err
		}
	}
	return nil
}

func generateTutorialNameSuffix() (string, error) {
	bytes := make([]byte, 3)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate tutorial database name: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}
