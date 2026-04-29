package cmd

import (
	"io"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
)

const mcpAllTarget = "all"

type mcpClientResultRow struct {
	Client string
	Status string
	Detail string
}

func outputMCPClientResultTable(w io.Writer, rows []mcpClientResultRow) error {
	table := tablewriter.NewTable(w,
		tablewriter.WithHeaderAlignment(tw.AlignLeft),
		tablewriter.WithPadding(tw.Padding{Left: "", Right: "  ", Overwrite: true}),
		tablewriter.WithRendition(tw.Rendition{
			Borders: tw.Border{
				Left:   tw.Off,
				Right:  tw.Off,
				Top:    tw.Off,
				Bottom: tw.Off,
			},
			Settings: tw.Settings{
				Separators: tw.Separators{
					ShowHeader:     tw.Off,
					ShowFooter:     tw.Off,
					BetweenRows:    tw.Off,
					BetweenColumns: tw.Off,
				},
				Lines: tw.Lines{ShowHeaderLine: tw.Off},
			},
		}),
	)
	table.Header("CLIENT", "STATUS", "DETAIL")
	for _, row := range rows {
		table.Append(row.Client, row.Status, row.Detail)
	}
	return table.Render()
}
