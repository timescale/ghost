package cmd

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/api/mock"
	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/tutorial"
)

func TestTutorialCmd(t *testing.T) {
	password := "testpass123"

	setupSuccessfulTutorial := func(m *mock.MockClientWithResponsesInterface, includeDeletes bool) {
		originalDatabase := sampleDatabase(func(db *api.Database) {
			db.Id = "orig1234567"
			db.Name = "tutorial-test"
			db.Password = &password
		})
		forkDatabase := sampleDatabase(func(db *api.Database) {
			db.Id = "fork1234567"
			db.Name = "tutorial-test-fork"
			db.Password = &password
		})

		// Step 1: ghost create --name tutorial-test --wait
		m.EXPECT().CreateDatabaseWithResponse(validCtx, "test-project", api.CreateDatabaseRequest{Name: new("tutorial-test")}).
			Return(&api.CreateDatabaseResponse{
				HTTPResponse: httpResponse(http.StatusAccepted),
				JSON202:      &originalDatabase,
			}, nil)

		// Step 4: ghost fork tutorial-test --name tutorial-test-fork --wait
		m.EXPECT().GetDatabaseWithResponse(validCtx, "test-project", "tutorial-test").
			Return(&api.GetDatabaseResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &originalDatabase,
			}, nil)
		m.EXPECT().ForkDatabaseWithResponse(validCtx, "test-project", "orig1234567", api.ForkDatabaseRequest{Name: new("tutorial-test-fork")}).
			Return(&api.ForkDatabaseResponse{
				HTTPResponse: httpResponse(http.StatusAccepted),
				JSON202:      &forkDatabase,
			}, nil)

		if includeDeletes {
			// Step 7a: ghost delete tutorial-test-fork --confirm
			m.EXPECT().GetDatabaseWithResponse(validCtx, "test-project", "tutorial-test-fork").
				Return(&api.GetDatabaseResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &forkDatabase,
				}, nil)
			m.EXPECT().DeleteDatabaseWithResponse(validCtx, "test-project", "fork1234567").
				Return(&api.DeleteDatabaseResponse{HTTPResponse: httpResponse(http.StatusAccepted)}, nil)

			// Step 7b: ghost delete tutorial-test --confirm
			m.EXPECT().GetDatabaseWithResponse(validCtx, "test-project", "tutorial-test").
				Return(&api.GetDatabaseResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &originalDatabase,
				}, nil)
			m.EXPECT().DeleteDatabaseWithResponse(validCtx, "test-project", "orig1234567").
				Return(&api.DeleteDatabaseResponse{HTTPResponse: httpResponse(http.StatusAccepted)}, nil)
		}
	}

	withTutorialStubs := func(t *testing.T) {
		t.Helper()
		t.Setenv("HOME", t.TempDir())

		originalGenerateNameSuffix := tutorialGenerateNameSuffix
		originalWaitForDatabaseWithProgress := common.WaitForDatabaseWithProgress
		originalExecuteQuery := common.ExecuteQuery

		tutorialGenerateNameSuffix = func() (string, error) {
			return "test", nil
		}
		common.WaitForDatabaseWithProgress = func(context.Context, io.Reader, io.Writer, common.WaitForDatabaseArgs) error {
			return nil
		}
		common.ExecuteQuery = func(_ context.Context, args common.ExecuteQueryArgs) (*common.QueryResult, error) {
			switch args.Query {
			case tutorial.SetupSQL:
				return &common.QueryResult{ResultSets: []common.ResultSet{
					{CommandTag: "CREATE TABLE"},
					{CommandTag: "INSERT 0 3"},
				}}, nil
			case tutorial.MutateForkSQL:
				return &common.QueryResult{ResultSets: []common.ResultSet{
					{CommandTag: "INSERT 0 1"},
					{CommandTag: "UPDATE 1"},
				}}, nil
			case tutorial.QuerySQL:
				rows := [][]string{
					{"1", "apples", "original"},
					{"2", "bananas", "original"},
					{"3", "carrots", "original"},
				}
				if args.DatabaseRef == "tutorial-test-fork" {
					rows = [][]string{
						{"1", "apples", "original"},
						{"2", "bananas", "fork"},
						{"3", "carrots", "original"},
						{"4", "dragonfruit", "fork"},
					}
				}
				return &common.QueryResult{ResultSets: []common.ResultSet{
					{
						Columns: []common.Column{
							{Name: "id"},
							{Name: "name"},
							{Name: "location"},
						},
						Rows: rows,
					},
				}}, nil
			default:
				return &common.QueryResult{}, nil
			}
		}

		t.Cleanup(func() {
			tutorialGenerateNameSuffix = originalGenerateNameSuffix
			common.WaitForDatabaseWithProgress = originalWaitForDatabaseWithProgress
			common.ExecuteQuery = originalExecuteQuery
		})
	}

	t.Run("non-interactive stdin", func(t *testing.T) {
		result := runCommand(t, []string{"tutorial"}, nil)

		if result.err == nil {
			t.Fatal("expected error, got nil")
		}
		assertOutput(t, result.err.Error(), "cannot run tutorial: stdin is not a terminal")
		assertOutput(t, result.stdout, "")
		assertOutput(t, result.stderr, "Error: cannot run tutorial: stdin is not a terminal\n")
	})

	t.Run("not logged in", func(t *testing.T) {
		withTutorialStubs(t)
		result := runCommand(t, []string{"tutorial"}, nil, withStdin("\n"), withIsTerminal(true), withClientError(errors.New("authentication required: no credentials found")))

		if result.err == nil {
			t.Fatal("expected error, got nil")
		}
		assertOutput(t, result.err.Error(), "authentication required: no credentials found")
		assertOutput(t, result.stdout, "")
		assertOutput(t, result.stderr, "Error: authentication required: no credentials found\n")
	})

	t.Run("read only config", func(t *testing.T) {
		withTutorialStubs(t)
		result := runCommand(t, []string{"tutorial"}, nil, withStdin("\n"), withIsTerminal(true), withEnv("GHOST_READ_ONLY", "true"))

		if result.err == nil {
			t.Fatal("expected error, got nil")
		}
		assertOutput(t, result.err.Error(), "cannot run tutorial while read_only is enabled; run `ghost config set read_only false` to allow tutorial writes")
		assertOutput(t, result.stdout, "")
		assertOutput(t, result.stderr, "Error: cannot run tutorial while read_only is enabled; run `ghost config set read_only false` to allow tutorial writes\n")
	})

	t.Run("keep tutorial databases", func(t *testing.T) {
		withTutorialStubs(t)
		result := runCommand(t, []string{"tutorial"}, func(m *mock.MockClientWithResponsesInterface) {
			setupSuccessfulTutorial(m, false)
		}, withStdin(strings.Repeat("\n", 7)+"n\n"), withIsTerminal(true))

		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		assertOutput(t, result.stdout, tutorialKeepExpectedStdout)
		assertOutput(t, result.stderr, strings.Repeat("Press Enter to run this command...\n", 7)+"Delete the tutorial databases now? [Y/n] ")
	})

	t.Run("delete tutorial databases", func(t *testing.T) {
		withTutorialStubs(t)
		result := runCommand(t, []string{"tutorial"}, func(m *mock.MockClientWithResponsesInterface) {
			setupSuccessfulTutorial(m, true)
		}, withStdin(strings.Repeat("\n", 7)+"y\n\n\n"), withIsTerminal(true))

		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		assertOutput(t, result.stdout, tutorialDeleteExpectedStdout)
		assertOutput(t, result.stderr, strings.Repeat("Press Enter to run this command...\n", 7)+"Delete the tutorial databases now? [Y/n] "+strings.Repeat("Press Enter to run this command...\n", 2))
	})

	setupCancelDuringCreate := func(m *mock.MockClientWithResponsesInterface, includeCleanupDelete bool) {
		originalDatabase := sampleDatabase(func(db *api.Database) {
			db.Id = "orig1234567"
			db.Name = "tutorial-test"
			db.Password = &password
		})

		m.EXPECT().CreateDatabaseWithResponse(validCtx, "test-project", api.CreateDatabaseRequest{Name: new("tutorial-test")}).
			Return(&api.CreateDatabaseResponse{
				HTTPResponse: httpResponse(http.StatusAccepted),
				JSON202:      &originalDatabase,
			}, nil)

		if includeCleanupDelete {
			m.EXPECT().GetDatabaseWithResponse(validCtx, "test-project", "tutorial-test").
				Return(&api.GetDatabaseResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &originalDatabase,
				}, nil)
			m.EXPECT().DeleteDatabaseWithResponse(validCtx, "test-project", "orig1234567").
				Return(&api.DeleteDatabaseResponse{HTTPResponse: httpResponse(http.StatusAccepted)}, nil)
		}
	}

	t.Run("canceled during create, confirm cleanup", func(t *testing.T) {
		withTutorialStubs(t)
		common.WaitForDatabaseWithProgress = func(context.Context, io.Reader, io.Writer, common.WaitForDatabaseArgs) error {
			return context.Canceled
		}

		result := runCommand(t, []string{"tutorial"}, func(m *mock.MockClientWithResponsesInterface) {
			setupCancelDuringCreate(m, true)
		}, withStdin("\ny\n"), withIsTerminal(true))

		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		assertOutput(t, result.stdout, tutorialCancelConfirmExpectedStdout)
		assertOutput(t, result.stderr, tutorialCancelConfirmExpectedStderr)
	})

	t.Run("canceled during create, decline cleanup", func(t *testing.T) {
		withTutorialStubs(t)
		common.WaitForDatabaseWithProgress = func(context.Context, io.Reader, io.Writer, common.WaitForDatabaseArgs) error {
			return context.Canceled
		}

		result := runCommand(t, []string{"tutorial"}, func(m *mock.MockClientWithResponsesInterface) {
			setupCancelDuringCreate(m, false)
		}, withStdin("\nn\n"), withIsTerminal(true))

		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		assertOutput(t, result.stdout, tutorialCancelDeclineExpectedStdout)
		assertOutput(t, result.stderr, tutorialCancelDeclineExpectedStderr)
	})
}

// Note: the table rows below have significant trailing whitespace.
const tutorialCommonExpectedStdout = `Welcome to the Ghost tutorial!

This guided tour will run real Ghost commands to demonstrate the core workflow:
create a database, load data, fork it, change the fork, compare the results, and clean up.

Temporary database names
  original: tutorial-test
  fork:     tutorial-test-fork

Step 1 / Create a database
--------------------------
$ ghost create --name tutorial-test --wait
Created database 'tutorial-test'
ID: orig1234567
Connection: postgresql://tsdbadmin:testpass123@host.example.com:5432/tsdb?sslmode=require

Step 2 / Add sample data with SQL
---------------------------------
The sql command connects to the database and executes the query you provide.
$ ghost sql tutorial-test \
    "CREATE TABLE ghost_tutorial_items (id serial PRIMARY KEY, name text NOT NULL, location text NOT NULL);
     INSERT INTO ghost_tutorial_items (name, location) VALUES ('apples', 'original'), ('bananas', 'original'), ('carrots', 'original');"
CREATE TABLE
INSERT 0 3

Step 3 / Query the original database
------------------------------------
$ ghost sql tutorial-test "SELECT id, name, location FROM ghost_tutorial_items ORDER BY id;"
 id │ name    │ location 
────┼─────────┼──────────
 1  │ apples  │ original 
 2  │ bananas │ original 
 3  │ carrots │ original 
(3 rows)


Step 4 / Fork the database
--------------------------
Forking creates an independent copy you can safely experiment with.
$ ghost fork tutorial-test --name tutorial-test-fork --wait
Forked 'tutorial-test' → 'tutorial-test-fork'
ID: fork1234567
Connection: postgresql://tsdbadmin:testpass123@host.example.com:5432/tsdb?sslmode=require

Step 5 / Mutate the fork
------------------------
These changes are made only on the fork.
$ ghost sql tutorial-test-fork \
    "INSERT INTO ghost_tutorial_items (name, location) VALUES ('dragonfruit', 'fork');
     UPDATE ghost_tutorial_items SET location = 'fork' WHERE name = 'bananas';"
INSERT 0 1
UPDATE 1

Step 6 / Compare the original and the fork
------------------------------------------
First, query the original database:
$ ghost sql tutorial-test "SELECT id, name, location FROM ghost_tutorial_items ORDER BY id;"
 id │ name    │ location 
────┼─────────┼──────────
 1  │ apples  │ original 
 2  │ bananas │ original 
 3  │ carrots │ original 
(3 rows)


Now query the fork. Notice the extra row and updated value:
$ ghost sql tutorial-test-fork "SELECT id, name, location FROM ghost_tutorial_items ORDER BY id;"
 id │ name        │ location 
────┼─────────────┼──────────
 1  │ apples      │ original 
 2  │ bananas     │ fork     
 3  │ carrots     │ original 
 4  │ dragonfruit │ fork     
(4 rows)


`

const tutorialKeepExpectedStdout = tutorialCommonExpectedStdout + `
Keeping the tutorial databases.
To clean them up later, run:
  ghost delete tutorial-test-fork --confirm
  ghost delete tutorial-test --confirm
`

const tutorialDeleteExpectedStdout = tutorialCommonExpectedStdout + `
Step 7 / Delete the tutorial databases
--------------------------------------
$ ghost delete tutorial-test-fork --confirm
Deleted 'tutorial-test-fork' (fork1234567)
$ ghost delete tutorial-test --confirm
Deleted 'tutorial-test' (orig1234567)

Tutorial complete. You created, queried, forked, changed, compared, and deleted Ghost databases.
`

const tutorialCancelStepOneStdout = `Welcome to the Ghost tutorial!

This guided tour will run real Ghost commands to demonstrate the core workflow:
create a database, load data, fork it, change the fork, compare the results, and clean up.

Temporary database names
  original: tutorial-test
  fork:     tutorial-test-fork

Step 1 / Create a database
--------------------------
$ ghost create --name tutorial-test --wait
Created database 'tutorial-test'
ID: orig1234567
Connection: postgresql://tsdbadmin:testpass123@host.example.com:5432/tsdb?sslmode=require
`

const tutorialCancelConfirmExpectedStdout = tutorialCancelStepOneStdout + `$ ghost delete tutorial-test --confirm
Deleted 'tutorial-test' (orig1234567)
`

const tutorialCancelDeclineExpectedStdout = tutorialCancelStepOneStdout

const tutorialCancelCommonStderr = "Press Enter to run this command...\n" +
	"\nTutorial canceled.\n" +
	"\n1 tutorial database was created:\n" +
	"  tutorial-test\n" +
	"\nDelete it now? [Y/n] "

const tutorialCancelConfirmExpectedStderr = tutorialCancelCommonStderr + "\n"

const tutorialCancelDeclineExpectedStderr = tutorialCancelCommonStderr +
	"\nTo delete them later, run:\n" +
	"  ghost delete tutorial-test --confirm\n"
