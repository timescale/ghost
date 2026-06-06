package serve

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	ghostapi "github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/serve/api"
	"github.com/timescale/ghost/internal/serve/driver"
)

// defaultRole matches the role used by `ghost sql` / `ghost connect` / etc.
const defaultRole = "tsdbadmin"

// connectionStringForService resolves a ghost-api database, retrieves the
// password for the default role, and builds a Postgres connection string (DSN)
// for it. When readOnly is true the connection is opened with the
// tsdb_admin.read_only_connection GUC set, matching `ghost sql` under the
// read_only config. On failure it returns an [api.NormalizedError] with
// Connect:true so callers can route it through handleNewSessionError.
func connectionStringForService(ctx context.Context, client ghostapi.ClientWithResponsesInterface, projectID, serviceID string, readOnly bool) (string, error) {
	database, err := fetchDatabase(ctx, client, projectID, serviceID)
	if err != nil {
		return "", err
	}
	if err := common.CheckReady(database); err != nil {
		return "", connectErr("%v", err)
	}

	password, err := common.GetPassword(database, defaultRole)
	if err != nil {
		if errors.Is(err, common.ErrPasswordNotFound) {
			return "", connectErr("no password found for database %s; run `ghost password %s` or add an entry to ~/.pgpass", database.Name, database.Id)
		}
		return "", connectErr("retrieving password: %v", err)
	}

	connStr, err := common.BuildConnectionString(common.ConnectionStringArgs{
		Database: database,
		Role:     defaultRole,
		Password: password,
		ReadOnly: readOnly,
	})
	if err != nil {
		return "", connectErr("building connection string: %v", err)
	}
	return connStr, nil
}

// fetchDatabase loads the ghost-api Database record for the given reference.
func fetchDatabase(ctx context.Context, client ghostapi.ClientWithResponsesInterface, projectID, databaseRef string) (ghostapi.Database, error) {
	resp, err := client.GetDatabaseWithResponse(ctx, projectID, databaseRef)
	if err != nil {
		return ghostapi.Database{}, connectErr("fetching database: %v", err)
	}
	if resp.StatusCode() != http.StatusOK {
		if resp.JSONDefault != nil {
			return ghostapi.Database{}, connectErr("API error: %s", resp.JSONDefault.Message)
		}
		return ghostapi.Database{}, connectErr("API returned status %d", resp.StatusCode())
	}
	if resp.JSON200 == nil {
		return ghostapi.Database{}, connectErr("empty response from API")
	}
	return *resp.JSON200, nil
}

// connectErr builds an [api.NormalizedError] for failures that occur while
// resolving a database connection (before the query starts). Marking it as a
// connect error lets the existing handleNewSessionError path surface it to the
// widget the same way an actual connection failure would.
func connectErr(format string, args ...any) *api.NormalizedError {
	return &api.NormalizedError{
		Message: fmt.Sprintf(format, args...),
		Source:  driver.Timescale,
		Connect: true,
	}
}
