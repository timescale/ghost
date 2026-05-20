package common

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/timescale/ghost/internal/api"
)

// DatabaseCounts represents the number of databases in each status.
type DatabaseCounts struct {
	Queued      int `json:"queued,omitempty"`
	Configuring int `json:"configuring,omitempty"`
	Running     int `json:"running,omitempty"`
	Pausing     int `json:"pausing,omitempty"`
	Paused      int `json:"paused,omitempty"`
	Resuming    int `json:"resuming,omitempty"`
	Deleting    int `json:"deleting,omitempty"`
	Deleted     int `json:"deleted,omitempty"`
	Upgrading   int `json:"upgrading,omitempty"`
	Unstable    int `json:"unstable,omitempty"`
	Unknown     int `json:"unknown,omitempty"`
}

// Status represents space usage including compute, storage, and database counts.
type Status struct {
	ComputeMinutes      int64          `json:"compute_minutes"`
	ComputeLimitMinutes int64          `json:"compute_limit_minutes"`
	StorageMib          int64          `json:"storage_mib"`
	StorageLimitMib     int64          `json:"storage_limit_mib"`
	Databases           DatabaseCounts `json:"databases"`
	CostToDate          *float64       `json:"cost_to_date,omitempty"`
	EstimatedTotalCost  *float64       `json:"estimated_total_cost,omitempty"`
	BillingPeriodStart  *time.Time     `json:"billing_period_start,omitempty"`
	BillingPeriodEnd    *time.Time     `json:"billing_period_end,omitempty"`
	SpaceID             string         `json:"space_id"`
}

// FetchStatus fetches space usage and database counts from the API.
func FetchStatus(ctx context.Context, client api.ClientWithResponsesInterface, projectID string) (Status, error) {
	var spaceStatus *api.SpaceStatus
	var databases []api.DatabaseWithUsage

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		resp, err := client.SpaceStatusWithResponse(ctx, projectID)
		if err != nil {
			return fmt.Errorf("failed to get space status: %w", err)
		}
		if resp.StatusCode() != http.StatusOK {
			return ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
		}
		if resp.JSON200 == nil {
			return errors.New("empty response from API")
		}
		spaceStatus = resp.JSON200
		return nil
	})

	g.Go(func() error {
		resp, err := client.ListDatabasesWithResponse(ctx, projectID)
		if err != nil {
			return fmt.Errorf("failed to list databases: %w", err)
		}
		if resp.StatusCode() != http.StatusOK {
			return ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
		}
		if resp.JSON200 == nil {
			return errors.New("empty response from API")
		}
		databases = *resp.JSON200
		return nil
	})

	if err := g.Wait(); err != nil {
		return Status{}, err
	}

	// Tally databases by status
	var counts DatabaseCounts
	for _, db := range databases {
		switch db.Status {
		case api.DatabaseStatusQueued:
			counts.Queued++
		case api.DatabaseStatusConfiguring:
			counts.Configuring++
		case api.DatabaseStatusRunning:
			counts.Running++
		case api.DatabaseStatusPausing:
			counts.Pausing++
		case api.DatabaseStatusPaused:
			counts.Paused++
		case api.DatabaseStatusResuming:
			counts.Resuming++
		case api.DatabaseStatusDeleting:
			counts.Deleting++
		case api.DatabaseStatusDeleted:
			counts.Deleted++
		case api.DatabaseStatusUpgrading:
			counts.Upgrading++
		case api.DatabaseStatusUnstable:
			counts.Unstable++
		default:
			counts.Unknown++
		}
	}

	return Status{
		ComputeMinutes:      spaceStatus.ComputeMinutes,
		ComputeLimitMinutes: spaceStatus.ComputeLimitMinutes,
		StorageMib:          spaceStatus.StorageMib,
		StorageLimitMib:     spaceStatus.StorageLimitMib,
		Databases:           counts,
		CostToDate:          spaceStatus.CostToDate,
		EstimatedTotalCost:  spaceStatus.EstimatedTotalCost,
		BillingPeriodStart:  spaceStatus.BillingPeriodStart,
		BillingPeriodEnd:    spaceStatus.BillingPeriodEnd,
		SpaceID:             projectID,
	}, nil
}
