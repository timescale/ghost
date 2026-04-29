package common

import (
	"fmt"
	"net/url"

	"github.com/timescale/ghost/internal/api"
)

// ConnectionStringArgs contains arguments for building a connection string.
type ConnectionStringArgs struct {
	Database api.Database
	Role     string
	Password string // If empty, password will be omitted from the connection string.
	ReadOnly bool   // If true, includes the tsdb_admin.read_only_connection GUC as a startup parameter.
}

// BuildConnectionString builds a PostgreSQL connection string for a database.
func BuildConnectionString(args ConnectionStringArgs) (string, error) {
	host := args.Database.Host
	port := args.Database.Port
	dbName := "tsdb"

	var userInfo string
	if args.Password != "" {
		userInfo = fmt.Sprintf("%s:%s", args.Role, url.QueryEscape(args.Password))
	} else {
		userInfo = args.Role
	}

	// Timescale Cloud requires TLS; make that explicit so clients don't
	// silently fall back to libpq's default sslmode=prefer.
	query := "sslmode=require"

	// If read-only mode is enabled, set the tsdb_admin.read_only_connection
	// GUC as a startup parameter. This activates an immutable read-only
	// connection that cannot be disabled for the session.
	// Decoded: options=-c tsdb_admin.read_only_connection=true
	if args.ReadOnly {
		query += "&options=-c%20tsdb_admin.read_only_connection%3Dtrue"
	}

	return fmt.Sprintf("postgresql://%s@%s:%d/%s?%s", userInfo, host, port, dbName, query), nil
}
