package serve

import (
	"testing"

	"github.com/timescale/ghost/internal/serve/dbdriver"
)

func TestPickResultSetToSurface(t *testing.T) {
	one := dbdriver.Column{Name: "n"}
	withCols := bufferedResultSet{columns: dbdriver.Columns{one}, rows: [][]any{{1}}}
	emptyCols := bufferedResultSet{columns: nil}
	otherCols := bufferedResultSet{columns: dbdriver.Columns{{Name: "m"}}, rows: [][]any{{2}}}

	tests := []struct {
		name string
		in   []bufferedResultSet
		want *bufferedResultSet
	}{
		{
			name: "empty input",
			in:   nil,
			want: nil,
		},
		{
			name: "single result with columns",
			in:   []bufferedResultSet{withCols},
			want: &withCols,
		},
		{
			name: "last result with columns wins",
			in:   []bufferedResultSet{withCols, otherCols},
			want: &otherCols,
		},
		{
			name: "last column-bearing result wins even when later results are column-less",
			in:   []bufferedResultSet{withCols, emptyCols},
			want: &withCols,
		},
		{
			name: "no columns anywhere falls back to last result",
			in:   []bufferedResultSet{emptyCols, emptyCols, emptyCols},
			want: &emptyCols,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := pickResultSetToSurface(tc.in)
			if (got == nil) != (tc.want == nil) {
				t.Fatalf("got=%v want=%v", got, tc.want)
			}
			if got == nil {
				return
			}
			if len(got.columns) != len(tc.want.columns) {
				t.Errorf("columns len = %d, want %d", len(got.columns), len(tc.want.columns))
			}
		})
	}
}
