// Package tutorial holds the data definitions for Ghost's guided tutorials.
// The CLI command in internal/cmd consumes these definitions to run a
// tutorial interactively; the generate-tutorial-docs binary consumes the
// same definitions to render the matching markdown docs. Keeping the data
// here (rather than next to either consumer) makes the source-of-truth
// explicit and avoids leaking doc-rendering concerns into the CLI package.
package tutorial

import (
	"fmt"
	"strconv"
	"strings"
)

// SQL statements run during the tutorial. Exported so test code in other
// packages can match recorded query strings against them.
const (
	SetupSQL      = "CREATE TABLE ghost_tutorial_items (id serial PRIMARY KEY, name text NOT NULL, location text NOT NULL); INSERT INTO ghost_tutorial_items (name, location) VALUES ('apples', 'original'), ('bananas', 'original'), ('carrots', 'original');"
	MutateForkSQL = "INSERT INTO ghost_tutorial_items (name, location) VALUES ('dragonfruit', 'fork'); UPDATE ghost_tutorial_items SET location = 'fork' WHERE name = 'bananas';"
	QuerySQL      = "SELECT id, name, location FROM ghost_tutorial_items ORDER BY id;"
)

// Placeholder values used when rendering the markdown tutorial doc. The live
// `ghost tutorial` command never uses these — it generates a real suffix and
// reads real IDs from the API — but the markdown renderer needs
// concrete-looking values so the example output reads naturally.
const (
	docsOriginalDatabaseName = "tutorial-example"
	docsForkDatabaseName     = "tutorial-example-fork"
	docsOriginalDatabaseID   = "abc1234567"
	docsForkDatabaseID       = "def1234567"
	docsConnectionString     = "postgresql://tsdbadmin:<password>@<host>:5432/tsdb?sslmode=require"
)

// Target controls whether a Block appears in the live CLI run, the rendered
// markdown doc, or both. The zero value (TargetAll) means the block is
// visible everywhere, which is the common case.
type Target int

const (
	TargetAll Target = iota
	TargetCLIOnly
	TargetDocsOnly
)

// Block is one unit of tutorial content: optional prose followed by an
// optional ghost CLI command. ExpectedOutput is shown only in the markdown
// doc — the live CLI prints whatever the sub-command actually emits. Target
// lets a block be doc-only (e.g. the cleanup preamble that explains how the
// live tutorial transitions into Step 7) or CLI-only. CreatesDatabase and
// RemovesDatabase track side effects on the cleanup list for the live
// runtime; they are ignored by the markdown renderer.
type Block struct {
	Prose           string
	Args            []string
	ExpectedOutput  string
	Target          Target
	CreatesDatabase string
	RemovesDatabase string
}

// Step is a numbered group of Blocks under a single heading. When
// JoinedBlocks is true, adjacent blocks render flush against each other (no
// blank line between them) — used for tight sequences such as the paired
// delete commands at the end of the tutorial.
type Step struct {
	Title        string
	Blocks       []Block
	JoinedBlocks bool
}

// Tutorial bundles everything about one tutorial: the narrative shown in
// docs/tutorials/<Filename>, the steps run by the live `ghost tutorial`
// command, and an optional cleanup step the live CLI conditionally runs
// after a user prompt. New tutorials should be added to All().
type Tutorial struct {
	Filename   string
	Title      string
	Callout    string
	Intro      []string
	Steps      []Step
	DeleteStep Step
}

// All is the registry of every tutorial defined in this package. The
// generate-tutorial-docs binary iterates this list to render markdown docs;
// the live `ghost tutorial` CLI command picks one (currently always
// learn-the-basics).
func All() []Tutorial {
	return []Tutorial{
		BuildLearnTheBasicsTutorial(docsOriginalDatabaseName, docsForkDatabaseName),
	}
}

// BuildLearnTheBasicsTutorial constructs the learn-the-basics tutorial
// using the provided database names. The docs registry passes placeholder
// names so the rendered markdown reads consistently; the live CLI passes
// dynamically generated names so its sub-commands operate on real
// databases.
func BuildLearnTheBasicsTutorial(originalDatabaseName, forkDatabaseName string) Tutorial {
	return Tutorial{
		Filename: "learn-the-basics.md",
		Title:    "Learn the basics of Ghost",
		Callout:  "Run `ghost tutorial` to step through this tutorial live in the CLI.",
		Intro: []string{
			"This guided tour walks through the core Ghost workflow: create a database, load data, fork it, change the fork, compare the results, and clean up. Each step shows the exact `ghost` command the live tutorial runs and the output you can expect to see.",
			fmt.Sprintf("Throughout this guide, the temporary databases are named `%s` and `%s`. The live `ghost tutorial` command generates a random suffix instead.", originalDatabaseName, forkDatabaseName),
		},
		Steps:      buildLearnTheBasicsSteps(originalDatabaseName, forkDatabaseName),
		DeleteStep: buildLearnTheBasicsDeleteStep(originalDatabaseName, forkDatabaseName),
	}
}

func buildLearnTheBasicsSteps(originalDatabaseName, forkDatabaseName string) []Step {
	threeRowQueryOutput := "" +
		" id │ name    │ location \n" +
		"────┼─────────┼──────────\n" +
		" 1  │ apples  │ original \n" +
		" 2  │ bananas │ original \n" +
		" 3  │ carrots │ original \n" +
		"(3 rows)"

	return []Step{
		{
			Title: "Create a database",
			Blocks: []Block{
				{
					Args:            []string{"create", "--name", originalDatabaseName, "--wait"},
					CreatesDatabase: originalDatabaseName,
					ExpectedOutput: "Created database '" + originalDatabaseName + "'\n" +
						"ID: " + docsOriginalDatabaseID + "\n" +
						"Connection: " + docsConnectionString,
				},
			},
		},
		{
			Title: "Add sample data with SQL",
			Blocks: []Block{
				{
					Prose:          "The sql command connects to the database and executes the query you provide.",
					Args:           []string{"sql", originalDatabaseName, SetupSQL},
					ExpectedOutput: "CREATE TABLE\nINSERT 0 3",
				},
			},
		},
		{
			Title: "Query the original database",
			Blocks: []Block{
				{
					Args:           []string{"sql", originalDatabaseName, QuerySQL},
					ExpectedOutput: threeRowQueryOutput,
				},
			},
		},
		{
			Title: "Fork the database",
			Blocks: []Block{
				{
					Prose:           "Forking creates an independent copy you can safely experiment with.",
					Args:            []string{"fork", originalDatabaseName, "--name", forkDatabaseName, "--wait"},
					CreatesDatabase: forkDatabaseName,
					ExpectedOutput: "Forked '" + originalDatabaseName + "' → '" + forkDatabaseName + "'\n" +
						"ID: " + docsForkDatabaseID + "\n" +
						"Connection: " + docsConnectionString,
				},
			},
		},
		{
			Title: "Mutate the fork",
			Blocks: []Block{
				{
					Prose:          "These changes are made only on the fork.",
					Args:           []string{"sql", forkDatabaseName, MutateForkSQL},
					ExpectedOutput: "INSERT 0 1\nUPDATE 1",
				},
			},
		},
		{
			Title: "Compare the original and the fork",
			Blocks: []Block{
				{
					Prose:          "First, query the original database:",
					Args:           []string{"sql", originalDatabaseName, QuerySQL},
					ExpectedOutput: threeRowQueryOutput,
				},
				{
					Prose: "Now query the fork. Notice the extra row and updated value:",
					Args:  []string{"sql", forkDatabaseName, QuerySQL},
					ExpectedOutput: "" +
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

func buildLearnTheBasicsDeleteStep(originalDatabaseName, forkDatabaseName string) Step {
	return Step{
		Title:        "Delete the tutorial databases",
		JoinedBlocks: true,
		Blocks: []Block{
			{
				Prose:  "When the main steps finish, the live tutorial asks whether to delete the databases. To run the cleanup step yourself, use the following.",
				Target: TargetDocsOnly,
			},
			{
				Args:            []string{"delete", forkDatabaseName, "--confirm"},
				RemovesDatabase: forkDatabaseName,
				ExpectedOutput:  "Deleted '" + forkDatabaseName + "' (" + docsForkDatabaseID + ")",
			},
			{
				Args:            []string{"delete", originalDatabaseName, "--confirm"},
				RemovesDatabase: originalDatabaseName,
				ExpectedOutput:  "Deleted '" + originalDatabaseName + "' (" + docsOriginalDatabaseID + ")",
			},
		},
	}
}

// FilterBlocks returns the blocks visible to the given audience. Blocks
// whose Target is TargetAll always pass; otherwise Target must match
// audience.
func FilterBlocks(blocks []Block, audience Target) []Block {
	out := make([]Block, 0, len(blocks))
	for _, block := range blocks {
		if block.Target == TargetAll || block.Target == audience {
			out = append(out, block)
		}
	}
	return out
}

// FormatCommand builds the user-facing echo string from sub-command args.
// The sql command's query argument is rendered specially so multi-statement
// queries appear on multiple indented, quoted lines instead of as a single
// long line. Shared between the CLI step echo and the markdown code blocks
// so the two stay in sync.
func FormatCommand(args []string) string {
	if len(args) == 3 && args[0] == "sql" {
		return formatSQLCommand(args[1], args[2])
	}
	return "ghost " + strings.Join(args, " ")
}

func formatSQLCommand(databaseRef, query string) string {
	statements := splitSQLStatements(query)
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

func splitSQLStatements(query string) []string {
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
