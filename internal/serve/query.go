package serve

import (
	"context"
	"log/slog"
	"time"

	"github.com/timescale/ghost/internal/log"
	"github.com/timescale/ghost/internal/serve/api"
)

// bufSize is the size of the result buffer returned from [Session.Query].
const bufSize = 100

// Executes a query on the session and streams the results back via a channel.
// It is the caller's responsibility to fully drain the channel (even when the
// run has been canceled) or it could result in a leaked goroutine and database
// connection.
func (s *Session) Query(ctx context.Context, run *Run) <-chan api.Result {
	logger := log.FromContext(ctx)

	results := make(chan api.Result, bufSize)

	go func() {
		defer close(results)

		s.lock.Lock()
		defer s.lock.Unlock()

		var rowCount int
		start := time.Now()

		sendError := func(err error) {
			normalized := s.driver.NormalizeError(ctx, err)
			duration := time.Since(start)
			results <- api.ErrorResult{
				Success:  false,
				RowCount: rowCount,
				Elapsed:  api.NewDuration(duration),
				Error:    normalized,
			}

			// Mark the session as broken if there was a fatal error.
			if normalized.Fatal {
				s.SetBroken()
			}
		}

		// Check for context cancellation before starting query execution.
		// Helps prevent race conditions between custom cancellation logic
		// (i.e. sending a cancellation signal/request to a database server)
		// and the start of query execution.
		if err := ctx.Err(); err != nil {
			logger.Debug("Context canceled before query execution", slog.Any("error", err))
			sendError(err)
			return
		}

		if err := s.runStatements(ctx, run.LeadingStatements()); err != nil {
			sendError(err)
			return
		}

		logger.Debug("Querying database")
		rows, err := s.driver.Query(ctx, run.FinalQuery())
		if err != nil {
			logger.Debug("Error querying database", slog.Any("error", err))
			sendError(err)
			return
		}
		defer func() {
			if err := rows.Close(); err != nil {
				logger.Error("Error closing rows", slog.Any("error", err))
			}
		}()

		logger.Debug("Fetching result columns")
		columns, err := rows.Columns()
		if err != nil {
			logger.Debug("Error fetching result columns", slog.Any("error", err))
			sendError(err)
			return
		}

		results <- api.ColumnResult{
			RunID:   run.ID,
			Columns: columns,
		}

		logger.Debug("Scanning rows",
			slog.Any("columns", columns),
			slog.Any("scanTypes", columns.ScanTypes()),
		)
		targets := columns.ScanTargets()
		for rows.Next() {
			if err := rows.Scan(targets...); err != nil {
				logger.Debug("Error scanning row", slog.Any("error", err))
				sendError(err)
				return
			}
			row := targets.Values()
			results <- api.RowResult{
				Row: row,
			}
			rowCount++
		}
		if err := rows.Err(); err != nil {
			logger.Debug("Error iterating over rows", slog.Any("error", err))
			sendError(err)
			return
		}

		logger.Debug("Closing rows")
		if err := rows.Close(); err != nil {
			logger.Debug("Error closing rows", slog.Any("error", err))
			sendError(err)
			return
		}

		logger.Debug("Fetching the number of rows affected")
		rowsAffected, err := rows.RowsAffected(ctx)
		if err != nil {
			// The query completed successfully, so no need to return an
			// error to the user here. Just log it and return the results
			// without the number of rows affected.
			logger.Error("Error getting the number of rows affected", slog.Any("error", err))
		}

		logger.Debug("Query success",
			slog.Int("rows", rowCount),
		)
		duration := time.Since(start)
		results <- api.SuccessResult{
			Success:      true,
			RowCount:     rowCount,
			RowsAffected: rowsAffected,
			Elapsed:      api.NewDuration(duration),
		}
	}()
	return results
}

func (s *Session) runStatements(ctx context.Context, statements []string) error {
	for _, stmt := range statements {
		if err := s.runStatement(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Session) runStatement(ctx context.Context, query string) error {
	logger := log.FromContext(ctx)

	logger.Debug("Running statement",
		slog.String("statement", query),
	)
	rows, err := s.driver.Query(ctx, query)
	if err != nil {
		logger.Debug("Error running statement", slog.Any("error", err))
		return err
	}

	logger.Debug("Closing rows")
	if err := rows.Close(); err != nil {
		logger.Debug("Error closing rows", slog.Any("error", err))
		return err
	}
	return nil
}
