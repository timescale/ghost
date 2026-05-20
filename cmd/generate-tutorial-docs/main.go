package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/timescale/ghost/internal/cmd"
)

func main() {
	outDir := flag.String("out", "./docs/tutorials", "Output directory")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatal(err)
	}

	for _, doc := range cmd.AllTutorialDocs() {
		path := filepath.Join(*outDir, doc.Filename)
		if err := os.WriteFile(path, []byte(doc.Content), 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Generated %s\n", path)
	}
}
