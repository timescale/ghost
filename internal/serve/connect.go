package serve

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/serve/dbdriver"
)

// connectErr is a typed wrapper that carries a NormalizedError with
// Connect:true. Returned by openDriverForService when something goes wrong
// before the query starts (DB not found, not ready, missing password, TLS
// failure).
type connectErr struct {
	norm *dbdriver.NormalizedError
}

func (c *connectErr) Error() string { return c.norm.Message }
func (c *connectErr) Normalized() *dbdriver.NormalizedError { return c.norm }

func newConnectErr(format string, args ...any) *connectErr {
	return &connectErr{
		norm: &dbdriver.NormalizedError{
			Message: fmt.Sprintf(format, args...),
			Source:  "ghost",
			Connect: true,
		},
	}
}

// fetchDatabase loads the ghost-api Database record + ready check.
func fetchDatabase(ctx context.Context, client api.ClientWithResponsesInterface, projectID, databaseRef string) (api.Database, error) {
	resp, err := client.GetDatabaseWithResponse(ctx, projectID, databaseRef)
	if err != nil {
		return api.Database{}, newConnectErr("fetching database: %v", err)
	}
	if resp.StatusCode() != http.StatusOK {
		if resp.JSONDefault != nil {
			return api.Database{}, newConnectErr("ghost-api: %s", resp.JSONDefault.Message)
		}
		return api.Database{}, newConnectErr("ghost-api returned %d", resp.StatusCode())
	}
	if resp.JSON200 == nil {
		return api.Database{}, newConnectErr("empty response from ghost-api")
	}
	return *resp.JSON200, nil
}

// defaultRole matches the role used by ghost sql / connect / etc.
const defaultRole = "tsdbadmin"

// openDriverForService resolves a ghost-api database, retrieves the password
// for the default role, and opens a Postgres driver against it.
func openDriverForService(ctx context.Context, client api.ClientWithResponsesInterface, projectID, serviceID string) (dbdriver.Driver, error) {
	database, err := fetchDatabase(ctx, client, projectID, serviceID)
	if err != nil {
		return nil, err
	}
	if err := common.CheckReady(database); err != nil {
		return nil, newConnectErr("%v", err)
	}

	password, err := common.GetPassword(database, defaultRole)
	if err != nil {
		if errors.Is(err, common.ErrPasswordNotFound) {
			return nil, newConnectErr("no password found for database %s; run `ghost password %s` or add an entry to ~/.pgpass", database.Name, database.Id)
		}
		return nil, newConnectErr("retrieving password: %v", err)
	}

	connStr, err := common.BuildConnectionString(common.ConnectionStringArgs{
		Database: database,
		Role:     defaultRole,
		Password: password,
	})
	if err != nil {
		return nil, newConnectErr("building connection string: %v", err)
	}

	driver, err := dbdriver.OpenPostgresDSN(ctx, connStr)
	if err != nil {
		return nil, newConnectErr("connecting: %v", err)
	}
	return driver, nil
}
