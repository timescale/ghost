package cmd

import (
	"fmt"
	"strings"
)

// TutorialDoc is one rendered tutorial markdown file together with the
// filename it should be written to.
type TutorialDoc struct {
	Filename string
	Content  string
}

// AllTutorialDocs renders every tutorial in the registry. The
// generate-tutorial-docs binary writes each Content to <outDir>/Filename.
func AllTutorialDocs() []TutorialDoc {
	tutorials := allTutorials()
	docs := make([]TutorialDoc, len(tutorials))
	for i, t := range tutorials {
		docs[i] = TutorialDoc{
			Filename: t.filename,
			Content:  renderTutorialMarkdown(t),
		}
	}
	return docs
}

// renderTutorialMarkdown walks the tutorial struct and emits markdown. The
// renderer is content-agnostic: every piece of tutorial-specific text comes
// from the struct, so updating a tutorial only requires editing its
// definition.
func renderTutorialMarkdown(t tutorial) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# %s\n\n", t.title)
	if t.callout != "" {
		fmt.Fprintf(&sb, "> %s\n\n", t.callout)
	}
	writeTutorialParagraphs(&sb, t.intro)

	for i, step := range t.steps {
		writeTutorialStepMarkdown(&sb, i+1, step)
	}
	writeTutorialStepMarkdown(&sb, len(t.steps)+1, t.deleteStep)

	return strings.TrimRight(sb.String(), "\n") + "\n"
}

func writeTutorialParagraphs(sb *strings.Builder, paragraphs []string) {
	for _, p := range paragraphs {
		sb.WriteString(p)
		sb.WriteString("\n\n")
	}
}

func writeTutorialStepMarkdown(sb *strings.Builder, number int, step tutorialStep) {
	fmt.Fprintf(sb, "## Step %d — %s\n\n", number, step.title)

	visibleBlocks := filterTutorialBlocks(step.blocks, tutorialTargetDocsOnly)

	if step.joinedBlocks {
		for _, block := range visibleBlocks {
			if block.prose != "" {
				sb.WriteString(block.prose + "\n\n")
			}
		}
		commands := make([]string, 0, len(visibleBlocks))
		outputs := make([]string, 0, len(visibleBlocks))
		for _, block := range visibleBlocks {
			if len(block.args) == 0 {
				continue
			}
			commands = append(commands, formatTutorialCommand(block.args))
			if block.expectedOutput != "" {
				outputs = append(outputs, block.expectedOutput)
			}
		}
		if len(commands) > 0 {
			writeTutorialCodeBlock(sb, "bash", strings.Join(commands, "\n"))
		}
		if len(outputs) > 0 {
			writeTutorialCodeBlock(sb, "", strings.Join(outputs, "\n"))
		}
		return
	}

	for _, block := range visibleBlocks {
		if block.prose != "" {
			sb.WriteString(block.prose + "\n\n")
		}
		if len(block.args) > 0 {
			writeTutorialCodeBlock(sb, "bash", formatTutorialCommand(block.args))
			if block.expectedOutput != "" {
				writeTutorialCodeBlock(sb, "", block.expectedOutput)
			}
		}
	}
}

func writeTutorialCodeBlock(sb *strings.Builder, lang, content string) {
	sb.WriteString("```")
	sb.WriteString(lang)
	sb.WriteString("\n")
	sb.WriteString(content)
	sb.WriteString("\n```\n\n")
}
