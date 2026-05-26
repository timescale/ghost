package main

import (
	"fmt"
	"strings"

	"github.com/timescale/ghost/internal/tutorial"
)

// renderTutorialMarkdown walks a tutorial.Tutorial and emits markdown. The
// renderer is content-agnostic: every piece of tutorial-specific text comes
// from the struct, so updating a tutorial only requires editing its
// definition in the tutorial package.
func renderTutorialMarkdown(t tutorial.Tutorial) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# %s\n\n", t.Title)
	if t.Callout != "" {
		fmt.Fprintf(&sb, "> %s\n\n", t.Callout)
	}
	writeParagraphs(&sb, t.Intro)

	for i, step := range t.Steps {
		writeStepMarkdown(&sb, i+1, step)
	}
	writeStepMarkdown(&sb, len(t.Steps)+1, t.DeleteStep)

	return strings.TrimRight(sb.String(), "\n") + "\n"
}

func writeParagraphs(sb *strings.Builder, paragraphs []string) {
	for _, p := range paragraphs {
		sb.WriteString(p)
		sb.WriteString("\n\n")
	}
}

func writeStepMarkdown(sb *strings.Builder, number int, step tutorial.Step) {
	fmt.Fprintf(sb, "## Step %d — %s\n\n", number, step.Title)

	visibleBlocks := tutorial.FilterBlocks(step.Blocks, tutorial.TargetDocsOnly)

	if step.JoinedBlocks {
		for _, block := range visibleBlocks {
			if block.Prose != "" {
				sb.WriteString(block.Prose + "\n\n")
			}
		}
		commands := make([]string, 0, len(visibleBlocks))
		outputs := make([]string, 0, len(visibleBlocks))
		for _, block := range visibleBlocks {
			if len(block.Args) == 0 {
				continue
			}
			commands = append(commands, tutorial.FormatCommand(block.Args))
			if block.ExpectedOutput != "" {
				outputs = append(outputs, block.ExpectedOutput)
			}
		}
		if len(commands) > 0 {
			writeCodeBlock(sb, "bash", strings.Join(commands, "\n"))
		}
		if len(outputs) > 0 {
			writeCodeBlock(sb, "", strings.Join(outputs, "\n"))
		}
		return
	}

	for _, block := range visibleBlocks {
		if block.Prose != "" {
			sb.WriteString(block.Prose + "\n\n")
		}
		if len(block.Args) > 0 {
			writeCodeBlock(sb, "bash", tutorial.FormatCommand(block.Args))
			if block.ExpectedOutput != "" {
				writeCodeBlock(sb, "", block.ExpectedOutput)
			}
		}
	}
}

func writeCodeBlock(sb *strings.Builder, lang, content string) {
	sb.WriteString("```")
	sb.WriteString(lang)
	sb.WriteString("\n")
	sb.WriteString(content)
	sb.WriteString("\n```\n\n")
}
