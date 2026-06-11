package serve

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSchemaHandler_ParamValidation covers the query-param validation that
// runs before any client/database access. The auth path and the
// FetchDatabaseSchema error mapping require a live database connection and
// are exercised by integration testing, not here.
func TestSchemaHandler_ParamValidation(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{
			name:       "missing databaseId returns 400",
			query:      "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "blank databaseId returns 400",
			query:      "databaseId=",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "non-boolean internal returns 400",
			query:      "databaseId=db-1&internal=maybe",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "numeric internal is not a valid bool",
			query:      "databaseId=db-1&internal=2",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "non-boolean definitions returns 400",
			query:      "databaseId=db-1&definitions=maybe",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "non-boolean comments returns 400",
			query:      "databaseId=db-1&comments=maybe",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{}
			req := httptest.NewRequest(http.MethodGet, "/api/schema?"+tc.query, nil)
			rr := httptest.NewRecorder()

			h.schemaHandler(rr, req)

			if rr.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d\nbody: %s", rr.Code, tc.wantStatus, rr.Body.String())
			}
		})
	}
}
