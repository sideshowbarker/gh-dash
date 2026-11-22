package issuerow

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/constants"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/theme"
)

func TestRenderTitle(t *testing.T) {
	const title = "Crash on startup"
	tests := []struct {
		name      string
		issueType *data.IssueType
		prefix    string
	}{
		{name: "no type renders just the title", issueType: nil, prefix: ""},
		{name: "Bug", issueType: &data.IssueType{Name: "Bug", Color: "RED"}, prefix: "Bug"},
		{
			name:      "unknown color",
			issueType: &data.IssueType{Name: "Epic", Color: "TEAL"},
			prefix:    "Epic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := &Issue{
				Ctx: &context.ProgramContext{
					Config: &config.Config{Theme: &config.ThemeConfig{}},
					Theme:  *theme.DefaultTheme,
				},
				Data: data.IssueData{Title: title, IssueType: tt.issueType},
			}
			got := ansi.Strip(issue.renderTitle(false))
			if tt.prefix == "" {
				require.Equal(t, title, got)
				return
			}
			require.True(
				t,
				strings.HasSuffix(got, " "+title),
				"title missing at the end of %q",
				got,
			)
			require.Contains(t, strings.TrimSuffix(got, title), tt.prefix)
		})
	}
}

func solid(hex string) compat.AdaptiveColor {
	return compat.AdaptiveColor{Light: lipgloss.Color(hex), Dark: lipgloss.Color(hex)}
}

func TestRenderLinkedPrs(t *testing.T) {
	const (
		openSeq   = "38;2;0;255;0m"
		mergedSeq = "38;2;255;0;255m"
		draftSeq  = "38;2;128;128;128m"
	)
	open := data.LinkedPullRequest{State: "OPEN"}
	draft := data.LinkedPullRequest{State: "OPEN", IsDraft: true}
	merged := data.LinkedPullRequest{State: "MERGED"}

	tests := []struct {
		name     string
		prs      []data.LinkedPullRequest
		text     string
		colorSeq string
	}{
		{name: "no PRs renders the placeholder", prs: nil, text: constants.EmptyCell},
		{
			name:     "one open PR",
			prs:      []data.LinkedPullRequest{open},
			text:     "1",
			colorSeq: openSeq,
		},
		{
			name:     "open wins over merged and draft",
			prs:      []data.LinkedPullRequest{merged, draft, open},
			text:     "3",
			colorSeq: openSeq,
		},
		{
			name:     "draft wins over merged",
			prs:      []data.LinkedPullRequest{merged, draft},
			text:     "2",
			colorSeq: draftSeq,
		},
		{
			name:     "all merged",
			prs:      []data.LinkedPullRequest{merged, merged},
			text:     "2",
			colorSeq: mergedSeq,
		},
	}

	ctx := &context.ProgramContext{Theme: theme.Theme{FaintText: solid("#808080")}}
	ctx.Styles.Colors.OpenPR = solid("#00FF00")
	ctx.Styles.Colors.MergedPR = solid("#FF00FF")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := &Issue{
				Ctx: ctx,
				Data: data.IssueData{
					LinkedPrs: data.LinkedPullRequests{TotalCount: len(tt.prs), Nodes: tt.prs},
				},
			}
			out := issue.renderLinkedPrs()
			require.Equal(t, tt.text, ansi.Strip(out))
			if tt.colorSeq != "" {
				require.Contains(t, out, tt.colorSeq)
			}
		})
	}
}

func TestRenderNumComments(t *testing.T) {
	tests := []struct {
		name     string
		count    int
		expected string
	}{
		{name: "no comments renders the placeholder", count: 0, expected: constants.EmptyCell},
		{name: "comments render their count", count: 4, expected: "4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := &Issue{
				Ctx:  &context.ProgramContext{Theme: *theme.DefaultTheme},
				Data: data.IssueData{Comments: data.IssueComments{TotalCount: tt.count}},
			}
			require.Equal(t, tt.expected, ansi.Strip(issue.renderNumComments()))
		})
	}
}

// On a selected row, the pill's reset mustn't clear the row's background behind the title.
func TestRenderTitleKeepsSelectedBackground(t *testing.T) {
	ctx := &context.ProgramContext{
		Config: &config.Config{Theme: &config.ThemeConfig{}},
		Theme:  *theme.DefaultTheme,
	}
	ctx.Theme.SelectedBackground = solid("#006400")
	issue := &Issue{
		Ctx:  ctx,
		Data: data.IssueData{Title: "Crash", IssueType: &data.IssueType{Name: "Bug", Color: "RED"}},
	}

	out := issue.renderTitle(true)
	beforeTitle := out[:strings.LastIndex(out, "Crash")]
	lastReset := max(
		strings.LastIndex(beforeTitle, "\x1b[0m"),
		strings.LastIndex(beforeTitle, "\x1b[m"),
	)
	require.GreaterOrEqual(t, lastReset, 0, "expected the pill to end with a reset in %q", out)
	require.Contains(t, beforeTitle[lastReset:], "48;2;0;100;0",
		"no background is restored between the pill's reset and the title in %q", out)
}
