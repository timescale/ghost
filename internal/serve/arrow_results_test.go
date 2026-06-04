package serve

import (
	"testing"

	"github.com/timescale/ghost/internal/serve/dbdriver"
	"github.com/timescale/ghost/internal/serve/dbtypes"
)

// buildBatch appends n single-column string rows and returns the finalized
// record batch so its serialized size can be measured.
func buildBatch(t *testing.T, n int, value string) (int64, func()) {
	t.Helper()
	cols := dbdriver.Columns{{Name: "n", ScanType: dbtypes.StringType}}
	rb, err := NewRecordBuilder(cols)
	if err != nil {
		t.Fatalf("NewRecordBuilder: %v", err)
	}
	for range n {
		if err := rb.AppendRow([]any{value}); err != nil {
			t.Fatalf("AppendRow: %v", err)
		}
	}
	batch := rb.NewRecordBatch()
	newCount := newRecordRowCount(batch, int64(n))
	batch.Release()
	return newCount, rb.Release
}

func TestNewRecordRowCount(t *testing.T) {
	t.Run("small narrow rows grow toward the max", func(t *testing.T) {
		// 100 tiny rows are far under the 5 MiB target, so the next batch
		// should grow, but never more than 2x the previous count.
		got, release := buildBatch(t, 100, "x")
		defer release()
		if got > 200 {
			t.Errorf("row count = %d, want <= 200 (2x growth cap)", got)
		}
		if got < 100 {
			t.Errorf("row count = %d, want >= 100 (narrow rows should not shrink)", got)
		}
	})

	t.Run("never drops below the floor", func(t *testing.T) {
		// A single huge row pushes bytes-per-row way over target; the next
		// count is clamped to the minimum rather than going to zero.
		huge := make([]byte, targetRecordBytes*2)
		for i := range huge {
			huge[i] = 'a'
		}
		got, release := buildBatch(t, 1, string(huge))
		defer release()
		if got != minRecordRowCount {
			t.Errorf("row count = %d, want %d (min floor)", got, minRecordRowCount)
		}
	})

	t.Run("zero previous count is a no-op", func(t *testing.T) {
		cols := dbdriver.Columns{{Name: "n", ScanType: dbtypes.StringType}}
		rb, err := NewRecordBuilder(cols)
		if err != nil {
			t.Fatalf("NewRecordBuilder: %v", err)
		}
		defer rb.Release()
		batch := rb.NewRecordBatch()
		defer batch.Release()
		if got := newRecordRowCount(batch, 0); got != 0 {
			t.Errorf("row count = %d, want 0", got)
		}
	})
}
