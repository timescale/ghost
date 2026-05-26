package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/timescale/ghost/internal/tutorial"
)

func TestAllTutorialDocsMatchGoldenFiles(t *testing.T) {
	for _, tut := range tutorial.All() {
		t.Run(tut.Filename, func(t *testing.T) {
			goldenPath := filepath.Join("..", "..", "docs", "tutorials", tut.Filename)
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("failed to read %s: %v", goldenPath, err)
			}
			got := renderTutorialMarkdown(tut)
			if diff := cmp.Diff(string(want), got); diff != "" {
				t.Errorf("%s out of date (run `go run ./cmd/generate-tutorial-docs`):\n%s", goldenPath, diff)
			}
		})
	}
}
