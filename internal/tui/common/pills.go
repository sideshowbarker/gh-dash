package common

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// WrapPills lays out the non-empty pills in rows no wider than width, with a space between pills, and a blank line
// between rows -- so a pill never sits right on top of another. A pill wider than width gets a row of its own.
func WrapPills(pills []string, width int) string {
	rows := []string{}
	row := ""
	for _, pill := range pills {
		switch {
		case pill == "":
			continue
		case row == "":
			row = pill
		case lipgloss.Width(row)+1+lipgloss.Width(pill) <= width:
			row += " " + pill
		default:
			rows = append(rows, row)
			row = pill
		}
	}
	if row != "" {
		rows = append(rows, row)
	}
	return strings.Join(rows, "\n\n")
}
