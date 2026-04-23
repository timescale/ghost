package common

import "testing"

func TestShareURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		token   string
		want    string
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShareURL(tt.baseURL, tt.token)
			if got != tt.want {
				t.Errorf("ShareURL(%q, %q) = %q, want %q", tt.baseURL, tt.token, got, tt.want)
			}
		})
	}
}
