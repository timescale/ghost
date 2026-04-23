package common

import "testing"

func TestShareURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		token   string
		want    string
		wantErr bool
	}{
		{
			name:    "no trailing slash",
			baseURL: "https://ghost.build/share",
			token:   "tok_xyz",
			want:    "https://ghost.build/share/tok_xyz",
		},
		{
			name:    "trailing slash on base",
			baseURL: "https://ghost.build/share/",
			token:   "tok_xyz",
			want:    "https://ghost.build/share/tok_xyz",
		},
		{
			name:    "token with characters requiring percent-encoding",
			baseURL: "https://ghost.build/share",
			token:   "tok with space",
			want:    "https://ghost.build/share/tok%20with%20space",
		},
		{
			name:    "invalid base URL",
			baseURL: "://not-a-url",
			token:   "tok_xyz",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ShareURL(tt.baseURL, tt.token)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ShareURL(%q, %q) error = %v, wantErr = %v", tt.baseURL, tt.token, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ShareURL(%q, %q) = %q, want %q", tt.baseURL, tt.token, got, tt.want)
			}
		})
	}
}
