package issueview

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/issuerow"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/keys"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/theme"
)

func newTestModelWithComments(t *testing.T, numComments int) Model {
	t.Helper()
	cfg, err := config.ParseConfig(config.Location{
		ConfigFlag: "../../../config/testdata/test-config.yml",
	})
	if err != nil {
		t.Fatal(err)
	}
	thm := theme.ParseTheme(&cfg)
	ctx := &context.ProgramContext{
		Config:            &cfg,
		Theme:             thm,
		Styles:            context.InitStyles(thm),
		MainContentHeight: 50,
	}

	comments := make([]data.IssueComment, numComments)
	for i := 0; i < numComments; i++ {
		comments[i] = data.IssueComment{
			Body:      "Comment body " + string(rune('1'+i)),
			UpdatedAt: time.Now().Add(time.Duration(i) * time.Hour),
		}
		comments[i].Author.Login = "user" + string(rune('1'+i))
	}

	m := NewModel(ctx)
	m.ctx = ctx
	m.issue = &issuerow.Issue{
		Ctx: ctx,
		Data: data.IssueData{
			Title:    "Test Issue",
			Url:      "https://github.com/test/repo/issues/1",
			Comments: data.IssueComments{Nodes: comments},
		},
	}
	return m
}

func TestCommentNavMode_EnterAndExit(t *testing.T) {
	m := newTestModelWithComments(t, 3)

	// Initially not in comment nav mode
	require.False(t, m.IsCommentNavMode())

	// Enter comment nav mode
	m.EnterCommentNavMode()
	require.True(t, m.IsCommentNavMode())
	require.Equal(t, 0, m.GetSelectedCommentIndex())

	// Exit comment nav mode
	m.ExitCommentNavMode()
	require.False(t, m.IsCommentNavMode())
	require.Equal(t, -1, m.GetSelectedCommentIndex())
}

func TestCommentNavMode_NoCommentsNoEnter(t *testing.T) {
	m := newTestModelWithComments(t, 0)

	// Try to enter comment nav mode
	m.EnterCommentNavMode()

	// Should not enter since no comments
	require.False(t, m.IsCommentNavMode())
}

func TestCommentNavigation_NextAndPrev(t *testing.T) {
	m := newTestModelWithComments(t, 3)
	m.EnterCommentNavMode()

	// Initial position is 0
	require.Equal(t, 0, m.GetSelectedCommentIndex())

	// Navigate forward
	m.SelectNextComment()
	require.Equal(t, 1, m.GetSelectedCommentIndex())

	m.SelectNextComment()
	require.Equal(t, 2, m.GetSelectedCommentIndex())

	// Can't go past last comment
	m.SelectNextComment()
	require.Equal(t, 2, m.GetSelectedCommentIndex())

	// Navigate backward
	m.SelectPrevComment()
	require.Equal(t, 1, m.GetSelectedCommentIndex())

	m.SelectPrevComment()
	require.Equal(t, 0, m.GetSelectedCommentIndex())

	// Can't go before first comment
	m.SelectPrevComment()
	require.Equal(t, 0, m.GetSelectedCommentIndex())
}

func TestGetSelectedComment(t *testing.T) {
	m := newTestModelWithComments(t, 2)

	// No comment selected yet
	require.Nil(t, m.GetSelectedComment())

	// Enter nav mode and select
	m.EnterCommentNavMode()
	comment := m.GetSelectedComment()
	require.NotNil(t, comment)
	require.Equal(t, "user1", comment.Author.Login)

	m.SelectNextComment()
	comment = m.GetSelectedComment()
	require.NotNil(t, comment)
	require.Equal(t, "user2", comment.Author.Login)
}

func TestSetRow_PreservesStateForSameIssue(t *testing.T) {
	m := newTestModelWithComments(t, 3)
	m.EnterCommentNavMode()
	m.SelectNextComment()

	require.True(t, m.IsCommentNavMode())
	require.Equal(t, 1, m.GetSelectedCommentIndex())

	// Set same issue again (same URL)
	issueData := m.issue.Data
	m.SetRow(&issueData)

	// State should be preserved
	require.True(t, m.IsCommentNavMode())
	require.Equal(t, 1, m.GetSelectedCommentIndex())
}

func TestSetRow_ResetsStateForDifferentIssue(t *testing.T) {
	m := newTestModelWithComments(t, 3)
	m.EnterCommentNavMode()
	m.SelectNextComment()

	require.True(t, m.IsCommentNavMode())
	require.Equal(t, 1, m.GetSelectedCommentIndex())

	// Set different issue
	newIssueData := data.IssueData{
		Title:    "Different Issue",
		Url:      "https://github.com/test/repo/issues/2",
		Comments: data.IssueComments{Nodes: []data.IssueComment{}},
	}
	m.SetRow(&newIssueData)

	// State should be reset
	require.False(t, m.IsCommentNavMode())
	require.Equal(t, -1, m.GetSelectedCommentIndex())
}

func TestGetCommentScrollPercent(t *testing.T) {
	m := newTestModelWithComments(t, 4)

	// No comment selected
	require.Equal(t, float64(-1), m.GetCommentScrollPercent())

	m.EnterCommentNavMode()

	// First comment
	percent := m.GetCommentScrollPercent()
	require.Greater(t, percent, 0.0)
	require.Less(t, percent, 1.0)

	// Move through comments, percent should increase
	prevPercent := percent
	m.SelectNextComment()
	percent = m.GetCommentScrollPercent()
	require.Greater(t, percent, prevPercent)
}

func TestGetNumComments(t *testing.T) {
	m := newTestModelWithComments(t, 3)
	require.Equal(t, 3, m.GetNumComments())

	m = newTestModelWithComments(t, 0)
	require.Equal(t, 0, m.GetNumComments())
}

func TestCommentNavMode_KeysReturnCorrectActions(t *testing.T) {
	m := newTestModelWithComments(t, 3)
	m.EnterCommentNavMode()

	// Tab key should return EnterCommentNavMode action (which toggles off in comment nav mode)
	msg := tea.KeyMsg{Type: tea.KeyTab}
	_, _, action := m.Update(msg)
	require.Nil(t, action) // Tab exits mode, returns nil

	// Re-enter mode for more tests
	m.EnterCommentNavMode()

	// Quote reply key
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}
	_, _, action = m.Update(msg)
	require.NotNil(t, action)
	require.Equal(t, IssueActionQuoteReply, action.Type)
}

func TestEnterCommentNavMode_Action(t *testing.T) {
	m := newTestModelWithComments(t, 3)

	// Tab key should return EnterCommentNavMode action when not in comment nav mode
	msg := tea.KeyMsg{Type: tea.KeyTab}
	_, _, action := m.Update(msg)

	require.NotNil(t, action)
	require.Equal(t, IssueActionEnterCommentNavMode, action.Type)
}

func TestUpdateWithReboundCommentNavKeys(t *testing.T) {
	// Save original key bindings
	originalQuoteReplyKeys := keys.IssueKeys.QuoteReply.Keys()

	// Rebind quote reply key to "r"
	keys.IssueKeys.QuoteReply.SetKeys("r")
	defer func() {
		// Restore original bindings
		keys.IssueKeys.QuoteReply.SetKeys(originalQuoteReplyKeys...)
	}()

	m := newTestModelWithComments(t, 3)
	m.EnterCommentNavMode()

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")}
	_, _, action := m.Update(msg)

	require.NotNil(t, action)
	require.Equal(t, IssueActionQuoteReply, action.Type)
}
