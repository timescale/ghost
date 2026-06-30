package mcp

import (
	"testing"
)

func TestDecodeImageDataURL(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantNil   bool
		wantErr   bool
		wantMIME  string
		wantBytes string
	}{
		{
			name:    "empty returns nil",
			input:   "",
			wantNil: true,
		},
		{
			name:      "valid png data url",
			input:     "data:image/png;base64,aGVsbG8=",
			wantMIME:  "image/png",
			wantBytes: "hello",
		},
		{
			name:      "data url with extra mime parameter",
			input:     "data:image/png;charset=utf-8;base64,aGVsbG8=",
			wantMIME:  "image/png;charset=utf-8",
			wantBytes: "hello",
		},
		{
			name:    "not a data url",
			input:   "http://example.com/x.png",
			wantErr: true,
		},
		{
			name:    "missing comma",
			input:   "data:image/png;base64",
			wantErr: true,
		},
		{
			name:    "not base64 encoded",
			input:   "data:image/png,rawdata",
			wantErr: true,
		},
		{
			name:    "invalid base64",
			input:   "data:image/png;base64,!!!notbase64!!!",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img, err := decodeImageDataURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if img != nil {
					t.Fatalf("expected nil image, got %+v", img)
				}
				return
			}
			if img == nil {
				t.Fatal("expected image, got nil")
			}
			if img.MIMEType != tt.wantMIME {
				t.Errorf("MIMEType = %q, want %q", img.MIMEType, tt.wantMIME)
			}
			if string(img.Data) != tt.wantBytes {
				t.Errorf("Data = %q, want %q", img.Data, tt.wantBytes)
			}
		})
	}
}
