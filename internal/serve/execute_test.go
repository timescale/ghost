package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow/ipc"

	"github.com/timescale/ghost/internal/serve/dbdriver"
	"github.com/timescale/ghost/internal/serve/dbtypes"
)

// fakeDriver is an in-memory dbdriver.Driver that returns a fixed result set
// for every Query. It records the statements it was asked to run so tests can
// assert multi-statement behavior.
type fakeDriver struct {
	cols    dbdriver.Columns
	rows    [][]any
	queries []string
}

func (d *fakeDriver) Ping(context.Context) error  { return nil }
func (d *fakeDriver) PingInterval() time.Duration { return 0 }
func (d *fakeDriver) Close() error                { return nil }
func (d *fakeDriver) Context(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(ctx)
}

func (d *fakeDriver) Query(_ context.Context, query string) (dbdriver.Rows, error) {
	d.queries = append(d.queries, query)
	return &fakeRows{cols: d.cols, rows: d.rows, idx: -1}, nil
}

func (d *fakeDriver) NormalizeError(_ context.Context, err error) *dbdriver.NormalizedError {
	return &dbdriver.NormalizedError{Message: err.Error(), Source: "fake"}
}

type fakeRows struct {
	cols dbdriver.Columns
	rows [][]any
	idx  int
}

func (r *fakeRows) Next() bool { r.idx++; return r.idx < len(r.rows) }
func (r *fakeRows) Scan(dest ...any) error {
	row := r.rows[r.idx]
	for i := range dest {
		reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(row[i]))
	}
	return nil
}
func (r *fakeRows) Err() error                                   { return nil }
func (r *fakeRows) Close() error                                 { return nil }
func (r *fakeRows) Columns() (dbdriver.Columns, error)           { return r.cols, nil }
func (r *fakeRows) RowsAffected(context.Context) (*int64, error) { return nil, nil }

func newTestServer() *Server {
	return &Server{
		runs:     newRunStore(),
		sessions: newSessionStore(),
		logger:   slog.New(slog.DiscardHandler),
	}
}

// runStreaming drives runQuery + handleArrowResults concurrently the way the
// widget does (executeQuery first, then arrowResults once the run is
// registered) and returns the decoded NDJSON lines plus the streamed arrow row
// count.
func runStreaming(t *testing.T, s *Server, req executeQueryRequest, driver dbdriver.Driver) (ndjson []map[string]any, arrowRows int64) {
	t.Helper()

	execW := httptest.NewRecorder()
	execR := httptest.NewRequest("POST", "/api/executeQuery", nil)
	done := make(chan struct{})
	go func() {
		s.runQuery(execW, execR, req, driver)
		close(done)
	}()

	// Wait for the run to be registered and its columns to be ready.
	var run *Run
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if run = s.runs.get(req.RunID); run != nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if run == nil {
		t.Fatal("run was never registered")
	}

	arrowW := httptest.NewRecorder()
	arrowR := httptest.NewRequest("POST", "/api/arrowResults", strings.NewReader(`{"runId":"`+req.RunID+`"}`))
	s.handleArrowResults(arrowW, arrowR)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runQuery did not return")
	}

	for _, line := range strings.Split(strings.TrimSpace(execW.Body.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("decoding NDJSON line %q: %v", line, err)
		}
		ndjson = append(ndjson, m)
	}

	if body := arrowW.Body.Bytes(); len(body) > 0 {
		rdr, err := ipc.NewReader(bytes.NewReader(body))
		if err != nil {
			t.Fatalf("opening arrow stream: %v", err)
		}
		defer rdr.Release()
		for rdr.Next() {
			arrowRows += rdr.RecordBatch().NumRows()
		}
		if err := rdr.Err(); err != nil {
			t.Fatalf("reading arrow stream: %v", err)
		}
	}
	return ndjson, arrowRows
}

func stringColumns(names ...string) dbdriver.Columns {
	cols := make(dbdriver.Columns, len(names))
	for i, n := range names {
		cols[i] = dbdriver.Column{Name: n, ScanType: dbtypes.StringType}
	}
	return cols
}

func TestRunQueryStreamsRows(t *testing.T) {
	s := newTestServer()
	driver := &fakeDriver{
		cols: stringColumns("n"),
		rows: [][]any{{"a"}, {"b"}, {"c"}},
	}
	req := executeQueryRequest{RunID: "run1", ProjectID: "p", ServiceID: "svc", Statements: []string{"SELECT n FROM t"}}

	ndjson, arrowRows := runStreaming(t, s, req, driver)

	if arrowRows != 3 {
		t.Errorf("streamed arrow rows = %d, want 3", arrowRows)
	}
	if len(ndjson) != 2 {
		t.Fatalf("NDJSON lines = %d, want 2 (columns, success): %v", len(ndjson), ndjson)
	}
	if _, ok := ndjson[0]["columns"]; !ok {
		t.Errorf("first NDJSON line should carry columns, got %v", ndjson[0])
	}
	last := ndjson[len(ndjson)-1]
	if last["success"] != true {
		t.Errorf("last NDJSON line should be success, got %v", last)
	}
	if last["rowCount"].(float64) != 3 {
		t.Errorf("rowCount = %v, want 3", last["rowCount"])
	}
	if s.runs.get("run1") != nil {
		t.Error("run should be deleted after runQuery returns")
	}
}

func TestRunQueryMultiStatementStreamsLast(t *testing.T) {
	s := newTestServer()
	driver := &fakeDriver{
		cols: stringColumns("n"),
		rows: [][]any{{"x"}, {"y"}},
	}
	req := executeQueryRequest{
		RunID:      "run2",
		ProjectID:  "p",
		ServiceID:  "svc",
		Statements: []string{"CREATE TEMP TABLE t (n text)", "INSERT INTO t VALUES ('x'),('y')", "SELECT n FROM t"},
	}

	ndjson, arrowRows := runStreaming(t, s, req, driver)

	if len(driver.queries) != 3 {
		t.Fatalf("driver ran %d statements, want 3: %v", len(driver.queries), driver.queries)
	}
	if arrowRows != 2 {
		t.Errorf("streamed arrow rows = %d, want 2", arrowRows)
	}
	last := ndjson[len(ndjson)-1]
	if last["executedStatements"].(float64) != 3 {
		t.Errorf("executedStatements = %v, want 3", last["executedStatements"])
	}
}

func TestArrowResultsRejectsConcurrentConsumers(t *testing.T) {
	s := newTestServer()
	run := &Run{
		id:    "run3",
		rows:  make(chan []any),
		ready: make(chan struct{}),
		done:  make(chan struct{}),
	}
	run.arrowStarted.Store(true) // simulate a consumer already streaming
	s.runs.add(run)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/arrowResults", strings.NewReader(`{"runId":"run3"}`))
	s.handleArrowResults(w, r)

	if w.Code != 409 {
		t.Errorf("status = %d, want 409 Conflict", w.Code)
	}
	var body jsonErrorBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body is not JSON: %v (%s)", err, w.Body.String())
	}
	if body.Error.Message == "" {
		t.Error("error body should carry a message")
	}
}
