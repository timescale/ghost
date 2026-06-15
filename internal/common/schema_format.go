package common

import (
	"fmt"
	"strings"
)

// FormatSchema formats a DatabaseSchema into a human-readable string,
// grouping objects under a SCHEMA: <name> header for each namespace. When
// includeDefinitions is false, the verbose object source bodies (view
// defining SELECTs and function/procedure bodies) are omitted, leaving just
// the structural summary (columns, constraints, indexes, signatures, etc.).
// When includeComments is true, object comments (COMMENT ON text) render as
// "-- " annotation lines under each object header and inline after columns.
func FormatSchema(schema *DatabaseSchema) string {
	var buf strings.Builder

	fmt.Fprintf(&buf, "DATABASE: %s (%s)\n", schema.Name, schema.ID)

	for _, ns := range schema.Schemas {
		fmt.Fprintf(&buf, "\nSCHEMA: %s\n", ns.Name)
		writeComment(&buf, ns.Comment)
		for _, table := range ns.Tables {
			fmt.Fprintf(&buf, "\nTABLE: %s\n", table.Name)
			writeComment(&buf, table.Comment)
			formatTableContents(&buf, table)
		}
		for _, view := range ns.Views {
			fmt.Fprintf(&buf, "\nVIEW: %s\n", view.Name)
			writeComment(&buf, view.Comment)
			formatViewContents(&buf, view)
		}
		for _, mv := range ns.MaterializedViews {
			fmt.Fprintf(&buf, "\nMATERIALIZED VIEW: %s\n", mv.Name)
			writeComment(&buf, mv.Comment)
			formatViewContents(&buf, mv)
		}
		for _, enum := range ns.Enums {
			fmt.Fprintf(&buf, "\nENUM: %s\n", enum.Name)
			writeComment(&buf, enum.Comment)
			formatEnumContents(&buf, enum)
		}
		for _, fn := range ns.Functions {
			fmt.Fprintf(&buf, "\nFUNCTION: %s\n", routineSignature(fn))
			writeComment(&buf, fn.Comment)
			formatRoutineContents(&buf, fn)
		}
		for _, proc := range ns.Procedures {
			fmt.Fprintf(&buf, "\nPROCEDURE: %s\n", routineSignature(proc))
			writeComment(&buf, proc.Comment)
			formatRoutineContents(&buf, proc)
		}
	}

	return buf.String()
}

// writeComment writes an object's COMMENT text as indented "-- " annotation
// lines directly under the object's header. Comments may span multiple
// lines; each line gets its own prefix. No-op when comments were not
// requested or the object has no comment.
func writeComment(buf *strings.Builder, comment string) {
	if comment == "" {
		return
	}
	for line := range strings.SplitSeq(comment, "\n") {
		fmt.Fprintf(buf, "  -- %s\n", line)
	}
}

// inlineComment renders a comment for same-line display (after a column
// entry), collapsing any newlines so the column list stays one line per
// column.
func inlineComment(comment string) string {
	return "  -- " + strings.ReplaceAll(comment, "\n", " ")
}

func formatTableContents(buf *strings.Builder, table TableSchema) {
	if table.Hypertable != nil {
		fmt.Fprintf(buf, "  -- HYPERTABLE (chunks=%d, compression=%s)\n",
			table.Hypertable.NumChunks,
			boolWord(table.Hypertable.CompressionEnabled, "enabled", "disabled"),
		)
	}
	if table.Foreign != nil {
		parts := append([]string{
			"server=" + table.Foreign.Server,
			"wrapper=" + table.Foreign.Wrapper,
		}, table.Foreign.Options...)
		fmt.Fprintf(buf, "  -- FOREIGN TABLE (%s)\n", strings.Join(parts, ", "))
	}

	singlePK := ""
	singleUnique := make(map[string]bool)
	singleFK := make(map[string]TableConstraint)
	singleCheck := make(map[string]CheckConstraint)
	fkCount := make(map[string]int)
	checkCount := make(map[string]int)
	var nonInlinedConstraints []TableConstraint
	var nonInlinedChecks []CheckConstraint

	for _, con := range table.Constraints {
		if len(con.Columns) == 1 {
			colName := con.Columns[0]
			switch con.Type {
			case ConstraintPrimaryKey:
				singlePK = colName
			case ConstraintUnique:
				singleUnique[colName] = true
			case ConstraintForeignKey:
				fkCount[colName]++
				if fkCount[colName] == 1 {
					singleFK[colName] = con
				} else {
					if fkCount[colName] == 2 {
						nonInlinedConstraints = append(nonInlinedConstraints, singleFK[colName])
						delete(singleFK, colName)
					}
					nonInlinedConstraints = append(nonInlinedConstraints, con)
				}
			}
		} else {
			nonInlinedConstraints = append(nonInlinedConstraints, con)
		}
	}

	for _, chk := range table.Checks {
		if len(chk.Columns) == 1 {
			colName := chk.Columns[0]
			checkCount[colName]++
			if checkCount[colName] == 1 {
				singleCheck[colName] = chk
			} else {
				if checkCount[colName] == 2 {
					nonInlinedChecks = append(nonInlinedChecks, singleCheck[colName])
					delete(singleCheck, colName)
				}
				nonInlinedChecks = append(nonInlinedChecks, chk)
			}
		} else {
			nonInlinedChecks = append(nonInlinedChecks, chk)
		}
	}

	maxNameLen := maxColumnNameLength(table.Columns)
	for _, col := range table.Columns {
		isPK := col.Name == singlePK
		isUnique := singleUnique[col.Name]
		fk, hasSingleFK := singleFK[col.Name]
		chk, hasSingleCheck := singleCheck[col.Name]
		line := formatTableColumn(col, maxNameLen, isPK, isUnique, hasSingleFK, fk, hasSingleCheck, chk)
		if col.Comment != "" {
			line += inlineComment(col.Comment)
		}
		fmt.Fprintf(buf, "  %s\n", line)
	}

	hasFollowup := len(nonInlinedConstraints) > 0 ||
		len(nonInlinedChecks) > 0 ||
		len(table.Exclusions) > 0 ||
		len(table.Indexes) > 0 ||
		len(table.Triggers) > 0 ||
		len(table.Partitions) > 0
	if hasFollowup {
		buf.WriteString("\n")
	}

	for _, con := range nonInlinedConstraints {
		fmt.Fprintf(buf, "  %s\n", formatConstraint(con))
	}
	for _, chk := range nonInlinedChecks {
		fmt.Fprintf(buf, "  %s\n", chk.Expression)
	}
	for _, exc := range table.Exclusions {
		fmt.Fprintf(buf, "  %s\n", exc.Definition)
	}
	for _, idx := range table.Indexes {
		fmt.Fprintf(buf, "  %s\n", formatIndex(idx))
	}
	for _, trg := range table.Triggers {
		fmt.Fprintf(buf, "  %s\n", formatTrigger(trg))
	}
	for _, part := range table.Partitions {
		// Schema-qualify the partition only when it lives in a different
		// schema than its parent table (PostgreSQL allows this).
		name := part.Name
		if part.Schema != "" {
			name = part.Schema + "." + part.Name
		}
		if part.Bound != "" {
			fmt.Fprintf(buf, "  PARTITION %s %s\n", name, part.Bound)
		} else {
			fmt.Fprintf(buf, "  PARTITION %s\n", name)
		}
	}
}

func formatTrigger(trg TriggerSchema) string {
	line := fmt.Sprintf("TRIGGER %s %s %s", trg.Name, trg.Timing, trg.Manipulation)
	if trg.Statement != "" {
		line += " " + trg.Statement
	}
	return line
}

func formatIndex(idx IndexSchema) string {
	var buf strings.Builder
	if idx.IsUnique {
		buf.WriteString("UNIQUE INDEX ")
	} else {
		buf.WriteString("INDEX ")
	}
	buf.WriteString(idx.Name)
	buf.WriteString(" (")
	buf.WriteString(idx.Columns)
	buf.WriteString(")")
	if idx.WhereClause != "" {
		buf.WriteString(" WHERE ")
		buf.WriteString(idx.WhereClause)
	}
	return buf.String()
}

func formatTableColumn(col TableColumnSchema, width int, isPK, isUnique, hasFK bool, fk TableConstraint, hasCheck bool, chk CheckConstraint) string {
	var parts []string

	displayType := strings.ToUpper(col.Type)
	showDefault := true
	isAutoGenerated := false

	switch {
	case col.IdentityType != "":
		isAutoGenerated = true
		showDefault = false
		parts = append(parts, strings.ToUpper(col.Type))
		if col.IdentityType == "a" {
			parts = append(parts, "GENERATED ALWAYS AS IDENTITY")
		} else {
			parts = append(parts, "GENERATED BY DEFAULT AS IDENTITY")
		}
	case col.IsSerial:
		isAutoGenerated = true
		showDefault = false
		switch col.Type {
		case "integer":
			displayType = "SERIAL"
		case "bigint":
			displayType = "BIGSERIAL"
		case "smallint":
			displayType = "SMALLSERIAL"
		}
		parts = append(parts, displayType)
	default:
		parts = append(parts, displayType)
	}

	if isPK {
		parts = append(parts, "PRIMARY KEY")
	}
	if col.NotNull && !isPK && !isAutoGenerated {
		parts = append(parts, "NOT NULL")
	}
	if isUnique {
		parts = append(parts, "UNIQUE")
	}
	if hasFK && len(fk.RefColumns) > 0 {
		parts = append(parts, fmt.Sprintf("REFERENCES %s(%s)", fk.RefTable, fk.RefColumns[0]))
	}
	if showDefault && col.Default != "" {
		parts = append(parts, "DEFAULT "+col.Default)
	}
	if hasCheck {
		parts = append(parts, chk.Expression)
	}

	return fmt.Sprintf("%-*s  %s", width, col.Name, strings.Join(parts, " "))
}

func formatConstraint(con TableConstraint) string {
	cols := strings.Join(con.Columns, ", ")
	switch con.Type {
	case ConstraintPrimaryKey:
		return fmt.Sprintf("PRIMARY KEY (%s)", cols)
	case ConstraintUnique:
		return fmt.Sprintf("UNIQUE (%s)", cols)
	case ConstraintForeignKey:
		refCols := strings.Join(con.RefColumns, ", ")
		return fmt.Sprintf("FOREIGN KEY (%s) REFERENCES %s(%s)", cols, con.RefTable, refCols)
	default:
		return ""
	}
}

func formatViewContents(buf *strings.Builder, view ViewSchema) {
	if view.ContinuousAggregate != nil {
		fmt.Fprintf(buf, "  -- CONTINUOUS AGGREGATE (materialized_only=%t, compression=%s)\n",
			view.ContinuousAggregate.MaterializedOnly,
			boolWord(view.ContinuousAggregate.CompressionEnabled, "enabled", "disabled"),
		)
	}
	maxNameLen := 0
	for _, col := range view.Columns {
		if len(col.Name) > maxNameLen {
			maxNameLen = len(col.Name)
		}
	}
	for _, col := range view.Columns {
		line := fmt.Sprintf("%-*s  %s", maxNameLen, col.Name, strings.ToUpper(col.Type))
		if col.Comment != "" {
			line += inlineComment(col.Comment)
		}
		fmt.Fprintf(buf, "  %s\n", line)
	}
	if len(view.Indexes) > 0 {
		buf.WriteString("\n")
		for _, idx := range view.Indexes {
			fmt.Fprintf(buf, "  %s\n", formatIndex(idx))
		}
	}
	if len(view.Triggers) > 0 {
		buf.WriteString("\n")
		for _, trg := range view.Triggers {
			fmt.Fprintf(buf, "  %s\n", formatTrigger(trg))
		}
	}
	if view.Definition != "" {
		buf.WriteString("\n  AS\n")
		for line := range strings.SplitSeq(view.Definition, "\n") {
			fmt.Fprintf(buf, "    %s\n", line)
		}
	}
}

func formatEnumContents(buf *strings.Builder, enum EnumSchema) {
	values := make([]string, len(enum.Values))
	for i, v := range enum.Values {
		values[i] = fmt.Sprintf("'%s'", v)
	}
	fmt.Fprintf(buf, "  %s\n", strings.Join(values, ", "))
}

// routineSignature renders a routine's display name including its identity
// argument list, so overloaded routines that share a name are
// distinguishable (e.g. "add(integer, integer)").
func routineSignature(r Routine) string {
	return fmt.Sprintf("%s(%s)", r.Name, r.Arguments)
}

func formatRoutineContents(buf *strings.Builder, r Routine) {
	if r.Definition == "" {
		return
	}
	for line := range strings.SplitSeq(r.Definition, "\n") {
		fmt.Fprintf(buf, "  %s\n", line)
	}
}

func maxColumnNameLength(columns []TableColumnSchema) int {
	max := 0
	for _, col := range columns {
		if len(col.Name) > max {
			max = len(col.Name)
		}
	}
	return max
}

func boolWord(b bool, yes, no string) string {
	if b {
		return yes
	}
	return no
}
