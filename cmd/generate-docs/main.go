package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra/doc"
	"github.com/timescale/ghost/internal/cmd"
)

func main() {
	out := flag.String("out", "./docs/cli", "Output directory")
	format := flag.String("format", "markdown", "Output format (markdown|man|rest|yaml)")
	front := flag.Bool("frontmatter", true, "Prepend YAML frontmatter")
	clean := flag.Bool("clean", false, "Remove output directory before generating")
	experimental := flag.Bool("experimental", false, "Include experimental commands (sets GHOST_EXPERIMENTAL)")
	flag.Parse()

	// Set GHOST_EXPERIMENTAL explicitly so an inherited value can't sneak
	// experimental commands into the generated docs.
	if *experimental {
		os.Setenv("GHOST_EXPERIMENTAL", "true")
	} else {
		os.Setenv("GHOST_EXPERIMENTAL", "false")
	}

	if *clean {
		if err := os.RemoveAll(*out); err != nil {
			log.Fatal(err)
		}
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}

	root, err := cmd.BuildRootCmd()
	if err != nil {
		log.Fatal(err)
	}
	root.DisableAutoGenTag = true // reproducible files without timestamps

	switch *format {
	case "markdown":
		if *front {
			prep := func(filename string) string {
				base := filepath.Base(filename)
				name := strings.TrimSuffix(base, filepath.Ext(base))
				title := strings.ReplaceAll(name, "_", " ")
				return fmt.Sprintf(
					"---\ntitle: %q\nslug: %q\ndescription: \"CLI reference for %s\"\n---\n\n",
					title, name, title)
			}
			link := func(name string) string { return strings.ToLower(name) }
			if err := doc.GenMarkdownTreeCustom(root, *out, prep, link); err != nil {
				log.Fatal(err)
			}
		} else {
			if err := doc.GenMarkdownTree(root, *out); err != nil {
				log.Fatal(err)
			}
		}
		// Cobra's markdown generator leaves a trailing blank line on every
		// file. Strip it so `git diff --check` doesn't flag the EOF.
		if err := trimTrailingBlankLines(*out, ".md"); err != nil {
			log.Fatal(err)
		}
	case "man":
		hdr := &doc.GenManHeader{Title: strings.ToUpper(root.Name()), Section: "1"}
		if err := doc.GenManTree(root, hdr, *out); err != nil {
			log.Fatal(err)
		}
	case "rest":
		if err := doc.GenReSTTree(root, *out); err != nil {
			log.Fatal(err)
		}
	case "yaml":
		if err := doc.GenYamlTree(root, *out); err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatalf("unknown format: %s", *format)
	}
}

func trimTrailingBlankLines(dir, ext string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ext {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		trimmed := append(bytes.TrimRight(data, "\n"), '\n')
		if bytes.Equal(trimmed, data) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(path, trimmed, info.Mode().Perm())
	})
}
