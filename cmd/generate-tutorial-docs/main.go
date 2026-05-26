package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/timescale/ghost/internal/tutorial"
)

func main() {
	outDir := flag.String("out", "./docs/tutorials", "Output directory")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatal(err)
	}

	for _, t := range tutorial.All() {
		path := filepath.Join(*outDir, t.Filename)
		if err := os.WriteFile(path, []byte(renderTutorialMarkdown(t)), 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Generated %s\n", path)
	}
}
