package common

import (
	"io"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
)

// NewTable returns a tablewriter.Table configured with Ghost's standard
// borderless, left-aligned style used for list and key/value command output.
//
// Header auto-formatting is disabled, so headers render exactly as the literal
// strings passed to Header() with no camel-case splitting or title-casing.
// Additional options may be supplied to extend or override the defaults.
func NewTable(w io.Writer, opts ...tablewriter.Option) *tablewriter.Table {
	defaults := []tablewriter.Option{
		tablewriter.WithHeaderAlignment(tw.AlignLeft),
		tablewriter.WithHeaderAutoFormat(tw.Off),
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
				Lines: tw.Lines{
					ShowHeaderLine: tw.Off,
				},
			},
		}),
	}
	return tablewriter.NewTable(w, append(defaults, opts...)...)
}
