package common

import "testing"

func TestShareURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		token   string
		dbName  string
		want    string
		wantErr bool
	}{
		{
			name:    "no trailing slash",
			baseURL: "https://ghost.build/share",
			token:   "tok_xyz",
			dbName:  "mydb",
			want:    "https://ghost.build/share/tok_xyz?name=mydb",
		},
		{
			name:    "trailing slash on base",
			baseURL: "https://ghost.build/share/",
			token:   "tok_xyz",
			dbName:  "mydb",
			want:    "https://ghost.build/share/tok_xyz?name=mydb",
		},
		{
			name:    "token with characters requiring percent-encoding",
			baseURL: "https://ghost.build/share",
			token:   "tok with space",
			dbName:  "mydb",
			want:    "https://ghost.build/share/tok%20with%20space?name=mydb",
		},
		{
			name:    "name with characters requiring percent-encoding",
			baseURL: "https://ghost.build/share",
			token:   "tok_xyz",
			dbName:  "my db & friends",
			want:    "https://ghost.build/share/tok_xyz?name=my+db+%26+friends",
		},
		{
			name:    "empty name omits the query parameter",
			baseURL: "https://ghost.build/share",
			token:   "tok_xyz",
			dbName:  "",
			want:    "https://ghost.build/share/tok_xyz",
		},
		{
			name:    "invalid base URL",
			baseURL: "://not-a-url",
			token:   "tok_xyz",
			dbName:  "mydb",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ShareURL(tt.baseURL, tt.token, tt.dbName)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ShareURL(%q, %q, %q) error = %v, wantErr = %v", tt.baseURL, tt.token, tt.dbName, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ShareURL(%q, %q, %q) = %q, want %q", tt.baseURL, tt.token, tt.dbName, got, tt.want)
			}
		})
	}
}
