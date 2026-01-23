package issueview

import (
	"sort"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/markdown"
	"github.com/dlvhdr/gh-dash/v4/internal/utils"
)

type RenderedActivity struct {
	UpdatedAt      time.Time
	OriginalIndex  int
	RenderedString string
}

func (m *Model) renderActivity() string {
	width := m.getIndentedContentWidth() - 2
	markdownRenderer := markdown.GetMarkdownRenderer(width)

	// First pass: collect comments with their original indices
	var activity []RenderedActivity
	for i, comment := range m.issue.Data.Comments.Nodes {
		activity = append(activity, RenderedActivity{
			UpdatedAt:     comment.UpdatedAt,
			OriginalIndex: i,
		})
	}

	// Sort by UpdatedAt (oldest first)
	sort.Slice(activity, func(i, j int) bool {
		return activity[i].UpdatedAt.Before(activity[j].UpdatedAt)
	})

	// Second pass: render comments in sorted order
	for sortedIdx := range activity {
		origIdx := activity[sortedIdx].OriginalIndex
		comment := m.issue.Data.Comments.Nodes[origIdx]
		isSelected := sortedIdx == m.selectedCommentIndex
		renderedComment, err := m.renderComment(comment, markdownRenderer, isSelected)
		if err != nil {
			continue
		}
		activity[sortedIdx].RenderedString = renderedComment
	}

	body := ""
	bodyStyle := lipgloss.NewStyle().PaddingLeft(2)
	if len(activity) == 0 {
		body = renderEmptyState()
	} else {
		var renderedActivities []string
		for _, activity := range activity {
			renderedActivities = append(renderedActivities, activity.RenderedString)
		}
		body = lipgloss.JoinVertical(lipgloss.Left, renderedActivities...)
	}

	return lipgloss.JoinVertical(lipgloss.Left, m.renderActivitiesTitle(), bodyStyle.Render(body))
}

func (m Model) renderActivitiesTitle() string {
	return m.ctx.Styles.Common.MainTextStyle.
		MarginBottom(1).
		Underline(true).
		Render(" Comments")
}

func renderEmptyState() string {
	return lipgloss.NewStyle().Italic(true).Render("No comments...")
}

func (m *Model) renderComment(comment data.IssueComment, markdownRenderer glamour.TermRenderer, isSelected bool) (string, error) {
	width := m.getIndentedContentWidth() - 2

	borderColor := m.ctx.Theme.FaintBorder
	if isSelected {
		borderColor = m.ctx.Theme.PrimaryBorder
	}

	header := lipgloss.NewStyle().
		Width(width).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).Render(
		lipgloss.JoinHorizontal(lipgloss.Top,
			m.ctx.Styles.Common.MainTextStyle.Render(comment.Author.Login),
			" ",
			lipgloss.NewStyle().Foreground(m.ctx.Theme.FaintText).Render(utils.TimeElapsed(comment.UpdatedAt)),
		))

	body := lineCleanupRegex.ReplaceAllString(comment.Body, "")
	body, err := markdownRenderer.Render(body)

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		body,
	), err
}
