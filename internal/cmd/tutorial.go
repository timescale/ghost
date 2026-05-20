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
	"strconv"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/util"
)

const (
	tutorialSetupSQL      = "CREATE TABLE ghost_tutorial_items (id serial PRIMARY KEY, name text NOT NULL, location text NOT NULL); INSERT INTO ghost_tutorial_items (name, location) VALUES ('apples', 'original'), ('bananas', 'original'), ('carrots', 'original');"
	tutorialMutateForkSQL = "INSERT INTO ghost_tutorial_items (name, location) VALUES ('dragonfruit', 'fork'); UPDATE ghost_tutorial_items SET location = 'fork' WHERE name = 'bananas';"
	tutorialQuerySQL      = "SELECT id, name, location FROM ghost_tutorial_items ORDER BY id;"

	// Placeholder values used when rendering the markdown tutorial doc. The
	// live `ghost tutorial` command never uses these — it generates a real
	// suffix and reads the real IDs from the API — but the markdown renderer
	// needs concrete-looking values so the example output reads naturally.
	tutorialDocsOriginalDatabaseName = "tutorial-example"
	tutorialDocsForkDatabaseName     = "tutorial-example-fork"
	tutorialDocsOriginalDatabaseID   = "abc1234567"
	tutorialDocsForkDatabaseID       = "def1234567"
	tutorialDocsConnectionString     = "postgresql://tsdbadmin:<password>@<host>:5432/tsdb?sslmode=require"
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

// tutorialTarget controls whether a block appears in the live CLI run, the
// rendered markdown doc, or both. The zero value (tutorialTargetAll) means
// the block is visible everywhere, which is the common case.
type tutorialTarget int

const (
	tutorialTargetAll tutorialTarget = iota
	tutorialTargetCLIOnly
	tutorialTargetDocsOnly
)

// tutorialBlock is one unit of tutorial content: optional prose followed by
// an optional ghost CLI command. expectedOutput is shown only in the markdown
// doc — the live CLI prints whatever the sub-command actually emits. target
// lets a block be doc-only (e.g. the cleanup preamble that explains how the
// live tutorial transitions into Step 7) or CLI-only. createsDatabase and
// removesDatabase track side effects on the cleanup list for the live
// runtime; they are ignored by the markdown renderer.
type tutorialBlock struct {
	prose           string
	args            []string
	expectedOutput  string
	target          tutorialTarget
	createsDatabase string
	removesDatabase string
}

// tutorialStep is a numbered group of blocks under a single heading. When
// joinedBlocks is true, adjacent blocks render flush against each other (no
// blank line between them) — used for tight sequences such as the paired
// delete commands at the end of the tutorial.
type tutorialStep struct {
	title        string
	blocks       []tutorialBlock
	joinedBlocks bool
}

// tutorial bundles everything about one tutorial: the narrative shown in
// docs/tutorials/<filename>, the steps run by the live `ghost tutorial`
// command, and an optional cleanup step the live CLI conditionally runs
// after a user prompt. New tutorials should be added to allTutorials().
type tutorial struct {
	filename   string
	title      string
	callout    string
	intro      []string
	steps      []tutorialStep
	deleteStep tutorialStep
}

// allTutorials is the registry of every tutorial defined in this package.
// AllTutorialDocs iterates this list to render markdown docs; the live
// `ghost tutorial` CLI command picks one (currently always learn-the-basics).
func allTutorials() []tutorial {
	return []tutorial{
		buildLearnTheBasicsTutorial(tutorialDocsOriginalDatabaseName, tutorialDocsForkDatabaseName),
	}
}

// buildLearnTheBasicsTutorial constructs the tutorial using the provided
// database names. The docs registry passes placeholder names so the
// rendered markdown reads consistently; the live CLI passes dynamically
// generated names so its sub-commands operate on real databases.
func buildLearnTheBasicsTutorial(originalDatabaseName, forkDatabaseName string) tutorial {
	return tutorial{
		filename: "learn-the-basics.md",
		title:    "Learn the basics of Ghost",
		callout:  "Run `ghost tutorial` to step through this tutorial live in the CLI.",
		intro: []string{
			"This guided tour walks through the core Ghost workflow: create a database, load data, fork it, change the fork, compare the results, and clean up. Each step shows the exact `ghost` command the live tutorial runs and the output you can expect to see.",
			fmt.Sprintf("Throughout this guide, the temporary databases are named `%s` and `%s`. The live `ghost tutorial` command generates a random suffix instead.", originalDatabaseName, forkDatabaseName),
		},
		steps:      buildTutorialSteps(originalDatabaseName, forkDatabaseName),
		deleteStep: buildTutorialDeleteStep(originalDatabaseName, forkDatabaseName),
	}
}

// filterTutorialBlocks returns the blocks visible to the given audience.
// Blocks whose target is tutorialTargetAll always pass; otherwise the
// target must match audience.
func filterTutorialBlocks(blocks []tutorialBlock, audience tutorialTarget) []tutorialBlock {
	out := make([]tutorialBlock, 0, len(blocks))
	for _, block := range blocks {
		if block.target == tutorialTargetAll || block.target == audience {
			out = append(out, block)
		}
	}
	return out
}

func buildTutorialSteps(originalDatabaseName, forkDatabaseName string) []tutorialStep {
	threeRowQueryOutput := "" +
		" id │ name    │ location \n" +
		"────┼─────────┼──────────\n" +
		" 1  │ apples  │ original \n" +
		" 2  │ bananas │ original \n" +
		" 3  │ carrots │ original \n" +
		"(3 rows)"

	return []tutorialStep{
		{
			title: "Create a database",
			blocks: []tutorialBlock{
				{
					args:            []string{"create", "--name", originalDatabaseName, "--wait"},
					createsDatabase: originalDatabaseName,
					expectedOutput: "Created database '" + originalDatabaseName + "'\n" +
						"ID: " + tutorialDocsOriginalDatabaseID + "\n" +
						"Connection: " + tutorialDocsConnectionString,
				},
			},
		},
		{
			title: "Add sample data with SQL",
			blocks: []tutorialBlock{
				{
					prose:          "The sql command connects to the database and executes the query you provide.",
					args:           []string{"sql", originalDatabaseName, tutorialSetupSQL},
					expectedOutput: "CREATE TABLE\nINSERT 0 3",
				},
			},
		},
		{
			title: "Query the original database",
			blocks: []tutorialBlock{
				{
					args:           []string{"sql", originalDatabaseName, tutorialQuerySQL},
					expectedOutput: threeRowQueryOutput,
				},
			},
		},
		{
			title: "Fork the database",
			blocks: []tutorialBlock{
				{
					prose:           "Forking creates an independent copy you can safely experiment with.",
					args:            []string{"fork", originalDatabaseName, "--name", forkDatabaseName, "--wait"},
					createsDatabase: forkDatabaseName,
					expectedOutput: "Forked '" + originalDatabaseName + "' → '" + forkDatabaseName + "'\n" +
						"ID: " + tutorialDocsForkDatabaseID + "\n" +
						"Connection: " + tutorialDocsConnectionString,
				},
			},
		},
		{
			title: "Mutate the fork",
			blocks: []tutorialBlock{
				{
					prose:          "These changes are made only on the fork.",
					args:           []string{"sql", forkDatabaseName, tutorialMutateForkSQL},
					expectedOutput: "INSERT 0 1\nUPDATE 1",
				},
			},
		},
		{
			title: "Compare the original and the fork",
			blocks: []tutorialBlock{
				{
					prose:          "First, query the original database:",
					args:           []string{"sql", originalDatabaseName, tutorialQuerySQL},
					expectedOutput: threeRowQueryOutput,
				},
				{
					prose: "Now query the fork. Notice the extra row and updated value:",
					args:  []string{"sql", forkDatabaseName, tutorialQuerySQL},
					expectedOutput: "" +
						" id │ name        │ location \n" +
						"────┼─────────────┼──────────\n" +
						" 1  │ apples      │ original \n" +
						" 2  │ bananas     │ fork     \n" +
						" 3  │ carrots     │ original \n" +
						" 4  │ dragonfruit │ fork     \n" +
						"(4 rows)",
				},
			},
		},
	}
}

func buildTutorialDeleteStep(originalDatabaseName, forkDatabaseName string) tutorialStep {
	return tutorialStep{
		title:        "Delete the tutorial databases",
		joinedBlocks: true,
		blocks: []tutorialBlock{
			{
				prose:  "When the main steps finish, the live tutorial asks whether to delete the databases. To run the cleanup step yourself, use the following.",
				target: tutorialTargetDocsOnly,
			},
			{
				args:            []string{"delete", forkDatabaseName, "--confirm"},
				removesDatabase: forkDatabaseName,
				expectedOutput:  "Deleted '" + forkDatabaseName + "' (" + tutorialDocsForkDatabaseID + ")",
			},
			{
				args:            []string{"delete", originalDatabaseName, "--confirm"},
				removesDatabase: originalDatabaseName,
				expectedOutput:  "Deleted '" + originalDatabaseName + "' (" + tutorialDocsOriginalDatabaseID + ")",
			},
		},
	}
}

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

	t := buildLearnTheBasicsTutorial(originalDatabaseName, forkDatabaseName)
	for i, step := range t.steps {
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
	if err := runTutorialStep(cmd, promptReader, len(t.steps)+1, t.deleteStep, &createdDatabaseNames); err != nil {
		return err
	}

	cmd.Println(tutorialSuccessStyle.Render("Tutorial complete. You created, queried, forked, changed, compared, and deleted Ghost databases."))
	return nil
}

func runTutorialStep(cmd *cobra.Command, promptReader *tutorialPromptReader, number int, step tutorialStep, createdDatabaseNames *[]string) error {
	printTutorialStep(cmd, number, step.title)
	visibleBlocks := filterTutorialBlocks(step.blocks, tutorialTargetCLIOnly)
	for i, block := range visibleBlocks {
		if block.prose != "" {
			cmd.Println(tutorialProseStyle.Render(block.prose))
		}
		if len(block.args) > 0 {
			if err := runTutorialCommand(cmd, promptReader, block.args); err != nil {
				return err
			}
		}
		if block.createsDatabase != "" {
			*createdDatabaseNames = append(*createdDatabaseNames, block.createsDatabase)
		}
		if block.removesDatabase != "" {
			*createdDatabaseNames = removeTutorialName(*createdDatabaseNames, block.removesDatabase)
		}
		isLast := i == len(visibleBlocks)-1
		if !step.joinedBlocks || isLast {
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
	printTutorialCommand(cmd, formatTutorialCommand(args))
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

// formatTutorialCommand builds the user-facing echo string from the args
// that will be passed to the sub-execution. The sql command's query argument
// is rendered specially so multi-statement queries appear on multiple
// indented, quoted lines instead of as a single long line.
func formatTutorialCommand(args []string) string {
	if len(args) == 3 && args[0] == "sql" {
		return formatTutorialSQLCommand(args[1], args[2])
	}
	return "ghost " + strings.Join(args, " ")
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

func formatTutorialSQLCommand(databaseRef, query string) string {
	statements := splitTutorialSQLStatements(query)
	if len(statements) <= 1 {
		return "ghost sql " + databaseRef + " " + strconv.Quote(query)
	}

	lines := []string{"ghost sql " + databaseRef + " \\"}
	for i, statement := range statements {
		quote := `"`
		if i > 0 {
			quote = " "
		}
		suffix := ";"
		if i == len(statements)-1 {
			suffix = `;"`
		}
		lines = append(lines, "  "+quote+statement+suffix)
	}
	return strings.Join(lines, "\n")
}

func splitTutorialSQLStatements(query string) []string {
	parts := strings.Split(query, ";")
	statements := make([]string, 0, len(parts))
	for _, part := range parts {
		statement := strings.TrimSpace(part)
		if statement != "" {
			statements = append(statements, statement)
		}
	}
	return statements
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
