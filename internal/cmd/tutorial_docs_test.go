package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAllTutorialDocsMatchGoldenFiles(t *testing.T) {
	for _, doc := range AllTutorialDocs() {
		t.Run(doc.Filename, func(t *testing.T) {
			goldenPath := filepath.Join("..", "..", "docs", "tutorials", doc.Filename)
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("failed to read %s: %v", goldenPath, err)
			}
			assertOutput(t, doc.Content, string(want))
		})
	}
}
