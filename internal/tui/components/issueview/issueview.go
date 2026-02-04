package issueview

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/common"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/autocomplete"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/inputbox"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/issuerow"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/constants"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/keys"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/markdown"
	"github.com/dlvhdr/gh-dash/v4/internal/utils"
)

var (
	htmlCommentRegex = regexp.MustCompile("(?U)<!--(.|[[:space:]])*-->")
	lineCleanupRegex = regexp.MustCompile(`((\n)+|^)([^\r\n]*\|[^\r\n]*(\n)?)+`)
)

type RepoLabelsFetchedMsg struct {
	Labels []data.Label
}

type RepoLabelsFetchFailedMsg struct {
	Err error
}

type Model struct {
	ctx       *context.ProgramContext
	issue     *issuerow.Issue
	sectionId int
	width     int

	ShowConfirmCancel    bool
	isCommenting         bool
	isLabeling           bool
	isAssigning          bool
	isUnassigning        bool
	isCommentNavMode     bool
	selectedCommentIndex int

	inputBox   inputbox.Model
	ac         *autocomplete.Model
	repoLabels []data.Label
}

func NewModel(ctx *context.ProgramContext) Model {
	inputBox := inputbox.NewModel(ctx)
	linesToAdjust := 5
	inputBox.SetHeight(common.InputBoxHeight - linesToAdjust)

	inputBox.OnSuggestionSelected = handleLabelSelection
	inputBox.CurrentContext = labelAtCursor
	inputBox.SuggestionsToExclude = allLabels

	ac := autocomplete.NewModel(ctx)
	inputBox.SetAutocomplete(&ac)

	return Model{
		issue: nil,

		isCommenting:         false,
		isLabeling:           false,
		isAssigning:          false,
		isUnassigning:        false,
		selectedCommentIndex: -1,

		inputBox:   inputBox,
		ac:         &ac,
		repoLabels: nil,
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, *IssueAction) {
	var (
		cmds  []tea.Cmd
		cmd   tea.Cmd
		taCmd tea.Cmd
	)

	switch msg := msg.(type) {
	case RepoLabelsFetchedMsg:
		clearCmd := m.ac.SetFetchSuccess()
		m.repoLabels = msg.Labels
		labelNames := data.GetLabelNames(msg.Labels)
		m.ac.SetSuggestions(labelNames)
		if m.isLabeling {
			cursorPos := m.inputBox.GetCursorPosition()
			currentLabel := labelAtCursor(cursorPos, m.inputBox.Value())
			existingLabels := allLabels(m.inputBox.Value())
			m.ac.Show(currentLabel, existingLabels)
		}
		return m, clearCmd, nil

	case RepoLabelsFetchFailedMsg:
		clearCmd := m.ac.SetFetchError(msg.Err)
		return m, clearCmd, nil

	case autocomplete.FetchSuggestionsRequestedMsg:
		// Only fetch when we're in labeling mode (where labels are relevant)
		if m.isLabeling {
			// If this is a forced refresh (e.g., via Ctrl+F), clear the cached labels
			// for this repo so FetchRepoLabels will actually call the gh CLI.
			if msg.Force {
				if m.issue != nil {
					repoName := m.issue.Data.GetRepoNameWithOwner()
					data.ClearRepoLabelCache(repoName)
				}
			}
			cmd := m.fetchLabels()
			return m, cmd, nil
		}
		return m, nil, nil

	case tea.KeyMsg:
		if m.isCommenting {
			switch msg.Type {
			case tea.KeyCtrlD:
				if len(strings.Trim(m.inputBox.Value(), " ")) != 0 {
					cmd = m.comment(m.inputBox.Value())
				}
				m.inputBox.Blur()
				m.isCommenting = false
				m.restoreInputBoxHeight()
				return m, cmd, nil

			case tea.KeyEsc, tea.KeyCtrlC:
				if !m.ShowConfirmCancel {
					m.shouldCancelComment()
				}
			default:
				if msg.String() == "Y" || msg.String() == "y" {
					if m.shouldCancelComment() {
						return m, nil, nil
					}
				}
				if m.ShowConfirmCancel && (msg.String() == "N" || msg.String() == "n") {
					m.inputBox.SetPrompt(constants.CommentPrompt)
					m.ShowConfirmCancel = false
					return m, nil, nil
				}
				m.inputBox.SetPrompt(constants.CommentPrompt)
				m.ShowConfirmCancel = false
			}

			m.inputBox, taCmd = m.inputBox.Update(msg)
			cmds = append(cmds, cmd, taCmd)
		} else if m.isLabeling {
			switch msg.Type {
			case tea.KeyCtrlD:
				labels := allLabels(m.inputBox.Value())
				if len(labels) > 0 {
					cmd = m.label(labels)
				}
				m.inputBox.Blur()
				m.isLabeling = false
				m.ac.Hide()
				return m, cmd, nil

			case tea.KeyEsc, tea.KeyCtrlC:
				m.inputBox.Blur()
				m.isLabeling = false
				m.ac.Hide()
				return m, nil, nil
			}

			if key.Matches(msg, autocomplete.RefreshSuggestionsKey) {
				if m.issue != nil {
					repoName := m.issue.Data.GetRepoNameWithOwner()
					data.ClearRepoLabelCache(repoName)
				}
				cmds = append(cmds, m.fetchLabels())
			}

			previousCursorPos := m.inputBox.GetCursorPosition()
			previousValue := m.inputBox.Value()
			previousLabel := labelAtCursor(previousCursorPos, previousValue)

			m.inputBox, taCmd = m.inputBox.Update(msg)
			cmds = append(cmds, cmd, taCmd)

			currentCursorPos := m.inputBox.GetCursorPosition()
			currentValue := m.inputBox.Value()
			currentLabel := labelAtCursor(currentCursorPos, currentValue)

			if currentLabel != previousLabel {
				existingLabels := allLabels(currentValue)
				m.ac.Show(currentLabel, existingLabels)
			}
		} else if m.isAssigning {
			switch msg.Type {
			case tea.KeyCtrlD:
				usernames := strings.Fields(m.inputBox.Value())
				if len(usernames) > 0 {
					cmd = m.assign(usernames)
				}
				m.inputBox.Blur()
				m.isAssigning = false
				return m, cmd, nil

			case tea.KeyEsc, tea.KeyCtrlC:
				m.inputBox.Blur()
				m.isAssigning = false
				return m, nil, nil
			}

			m.inputBox, taCmd = m.inputBox.Update(msg)
			cmds = append(cmds, cmd, taCmd)
		} else if m.isUnassigning {
			switch msg.Type {
			case tea.KeyCtrlD:
				usernames := strings.Fields(m.inputBox.Value())
				if len(usernames) > 0 {
					cmd = m.unassign(usernames)
				}
				m.inputBox.Blur()
				m.isUnassigning = false
				return m, cmd, nil

			case tea.KeyEsc, tea.KeyCtrlC:
				m.inputBox.Blur()
				m.isUnassigning = false
				return m, nil, nil
			}

			m.inputBox, taCmd = m.inputBox.Update(msg)
			cmds = append(cmds, cmd, taCmd)
		} else if m.isCommentNavMode {
			// Comment navigation mode: j/k navigate, q quotes, Esc/Tab exits
			switch {
			case key.Matches(msg, keys.IssueKeys.NextComment):
				m.SelectNextComment()
				return m, nil, nil
			case key.Matches(msg, keys.IssueKeys.PrevComment):
				m.SelectPrevComment()
				return m, nil, nil
			case key.Matches(msg, keys.IssueKeys.QuoteReply):
				return m, nil, &IssueAction{Type: IssueActionQuoteReply}
			case msg.Type == tea.KeyEsc, key.Matches(msg, keys.IssueKeys.EnterCommentNavMode):
				m.ExitCommentNavMode()
				return m, nil, nil
			}
			return m, nil, nil
		} else {
			switch {
			case key.Matches(msg, keys.IssueKeys.Label):
				return m, nil, &IssueAction{Type: IssueActionLabel}
			case key.Matches(msg, keys.IssueKeys.Assign):
				return m, nil, &IssueAction{Type: IssueActionAssign}
			case key.Matches(msg, keys.IssueKeys.Unassign):
				return m, nil, &IssueAction{Type: IssueActionUnassign}
			case key.Matches(msg, keys.IssueKeys.Comment):
				return m, nil, &IssueAction{Type: IssueActionComment}
			case key.Matches(msg, keys.IssueKeys.Close):
				return m, nil, &IssueAction{Type: IssueActionClose}
			case key.Matches(msg, keys.IssueKeys.Reopen):
				return m, nil, &IssueAction{Type: IssueActionReopen}
			case key.Matches(msg, keys.IssueKeys.EnterCommentNavMode):
				return m, nil, &IssueAction{Type: IssueActionEnterCommentNavMode}
			case key.Matches(msg, keys.IssueKeys.EditorComment):
				return m, nil, &IssueAction{Type: IssueActionEditorComment}
			}
			return m, nil, nil
		}
	}

	switch msg.(type) {
	case spinner.TickMsg, autocomplete.ClearFetchStatusMsg:
		var acCmd tea.Cmd
		*m.ac, acCmd = m.ac.Update(msg)
		cmds = append(cmds, acCmd)
	}

	return m, tea.Batch(cmds...), nil
}

func (m Model) View() string {
	s := strings.Builder{}

	s.WriteString(m.renderFullNameAndNumber())
	s.WriteString("\n")

	s.WriteString(m.renderTitle())
	s.WriteString("\n\n")
	s.WriteString(m.renderStatusPill())
	s.WriteString("\n\n")
	s.WriteString(m.renderAuthor())
	s.WriteString("\n\n")

	labels := m.renderLabels()
	if labels != "" {
		s.WriteString(labels)
		s.WriteString("\n\n")
	}

	s.WriteString(m.renderBody())
	s.WriteString("\n\n")
	s.WriteString(m.renderActivity())

	if m.isCommenting || m.isAssigning || m.isUnassigning {
		s.WriteString(m.inputBox.View())
	} else if m.isLabeling {
		s.WriteString(m.inputBox.ViewWithAutocomplete())
	}

	return lipgloss.NewStyle().Padding(0, m.ctx.Styles.Sidebar.ContentPadding).Render(s.String())
}

func (m *Model) renderFullNameAndNumber() string {
	return common.RenderPreviewHeader(m.ctx.Theme, m.width,
		fmt.Sprintf("#%d · %s", m.issue.Data.GetNumber(), m.issue.Data.GetRepoNameWithOwner()))
}

func (m *Model) renderTitle() string {
	return common.RenderPreviewTitle(m.ctx.Theme, m.ctx.Styles.Common, m.width, m.issue.Data.Title)
}

func (m *Model) renderStatusPill() string {
	bgColor := ""
	content := ""
	switch m.issue.Data.State {
	case "OPEN":
		bgColor = m.ctx.Styles.Colors.OpenIssue.Dark
		content = " Open"
	case "CLOSED":
		bgColor = m.ctx.Styles.Colors.ClosedIssue.Dark
		content = " Closed"
	}

	return m.ctx.Styles.PrView.PillStyle.
		BorderForeground(lipgloss.Color(bgColor)).
		Background(lipgloss.Color(bgColor)).
		Render(content)
}

func (m *Model) renderAuthor() string {
	authorAssociation := m.issue.Data.AuthorAssociation
	if authorAssociation == "" {
		authorAssociation = "unknown role"
	}
	time := lipgloss.NewStyle().Render(utils.TimeElapsed(m.issue.Data.CreatedAt))
	return lipgloss.JoinHorizontal(lipgloss.Top,
		" by ",
		lipgloss.NewStyle().Foreground(m.ctx.Theme.PrimaryText).Render(
			lipgloss.NewStyle().Bold(true).Render("@"+m.issue.Data.Author.Login)),
		lipgloss.NewStyle().Foreground(m.ctx.Theme.FaintText).Render(
			lipgloss.JoinHorizontal(lipgloss.Top, " ⋅ ", time, " ago", " ⋅ ")),
		lipgloss.NewStyle().Foreground(m.ctx.Theme.FaintText).Render(
			lipgloss.JoinHorizontal(lipgloss.Top, data.GetAuthorRoleIcon(m.issue.Data.AuthorAssociation,
				m.ctx.Theme), " ", lipgloss.NewStyle().Foreground(m.ctx.Theme.FaintText).Render(strings.ToLower(authorAssociation))),
		),
	)
}

func (m *Model) renderBody() string {
	width := m.getIndentedContentWidth()
	// Strip HTML comments from body and cleanup body.
	body := htmlCommentRegex.ReplaceAllString(m.issue.Data.Body, "")
	body = lineCleanupRegex.ReplaceAllString(body, "")

	body = strings.TrimSpace(body)
	if body == "" {
		return lipgloss.NewStyle().Italic(true).Foreground(m.ctx.Theme.FaintText).Render("No description provided.")
	}

	markdownRenderer := markdown.GetMarkdownRenderer(width)
	rendered, err := markdownRenderer.Render(body)
	if err != nil {
		return ""
	}

	return lipgloss.NewStyle().
		Width(width).
		MaxWidth(width).
		Align(lipgloss.Left).
		Render(rendered)
}

func (m *Model) renderLabels() string {
	width := m.getIndentedContentWidth()
	labels := m.issue.Data.Labels.Nodes
	style := m.ctx.Styles.PrView.PillStyle

	return common.RenderLabels(width, labels, style)
}

func (m *Model) getIndentedContentWidth() int {
	return m.width - 6
}

func (m *Model) SetWidth(width int) {
	m.width = width
	m.inputBox.SetWidth(width)
	m.ac.SetWidth(width - 4)
}

func (m *Model) SetSectionId(id int) {
	m.sectionId = id
}

func (m *Model) SetRow(data *data.IssueData) {
	// Only reset comment nav state if the issue actually changes
	isNewIssue := true
	if data != nil && m.issue != nil && m.issue.Data.Url == data.Url {
		isNewIssue = false
	}

	if data == nil {
		m.issue = nil
	} else {
		m.issue = &issuerow.Issue{Ctx: m.ctx, Data: *data}
	}

	if isNewIssue {
		m.selectedCommentIndex = -1
		m.isCommentNavMode = false
	}
}

func (m *Model) IsTextInputBoxFocused() bool {
	return m.isCommenting || m.isAssigning || m.isUnassigning || m.isLabeling
}

func (m *Model) IsCommentNavMode() bool {
	return m.isCommentNavMode
}

func (m *Model) EnterCommentNavMode() {
	if m.issue == nil || m.GetNumComments() == 0 {
		return
	}
	m.isCommentNavMode = true
	// Select first comment if none selected
	if m.selectedCommentIndex < 0 {
		m.selectedCommentIndex = 0
	}
}

func (m *Model) ExitCommentNavMode() {
	m.isCommentNavMode = false
	m.selectedCommentIndex = -1
}

// GetCommentScrollPercent returns the approximate scroll percentage to show the selected comment.
// Returns -1 if no comment is selected.
func (m *Model) GetCommentScrollPercent() float64 {
	if m.selectedCommentIndex < 0 {
		return -1
	}
	numComments := m.GetNumComments()
	if numComments == 0 {
		return -1
	}
	// Comments are at the bottom of the view, so we estimate based on comment position.
	// Assume the header/body takes about 30% of the view, comments take 70%.
	basePercent := 0.30
	commentPercent := 0.70 * (float64(m.selectedCommentIndex) / float64(numComments))
	return basePercent + commentPercent
}

func (m *Model) GetIsCommenting() bool {
	return m.isCommenting
}

func (m *Model) shouldCancelComment() bool {
	if !m.ShowConfirmCancel {
		m.inputBox.SetPrompt(lipgloss.NewStyle().Foreground(m.ctx.Theme.ErrorText).Render("Discard comment? (y/N)"))
		m.ShowConfirmCancel = true
		return false
	}
	m.inputBox.Blur()
	m.isCommenting = false
	m.ShowConfirmCancel = false
	m.restoreInputBoxHeight()
	return true
}

func (m *Model) SetIsCommenting(isCommenting bool) tea.Cmd {
	if m.issue == nil {
		return nil
	}

	if !m.isCommenting && isCommenting {
		m.inputBox.Reset()
		m.ac.Reset() // Clear any stale autocomplete state (e.g., from labeling)
		m.expandInputBoxForCommenting()
	}
	m.isCommenting = isCommenting
	m.inputBox.SetPrompt(constants.CommentPrompt)

	if isCommenting {
		return tea.Sequence(textarea.Blink, m.inputBox.Focus())
	}
	return nil
}

func (m *Model) expandInputBoxForCommenting() {
	// Set input box to about 75% of the main content height
	expandedHeight := int(float64(m.ctx.MainContentHeight) * 0.75)
	if expandedHeight < common.InputBoxHeight {
		expandedHeight = common.InputBoxHeight
	}
	m.inputBox.SetHeight(expandedHeight)
}

func (m *Model) restoreInputBoxHeight() {
	linesToAdjust := 5
	m.inputBox.SetHeight(common.InputBoxHeight - linesToAdjust)
}

func (m *Model) GetIsAssigning() bool {
	return m.isAssigning
}

func (m *Model) SetIsAssigning(isAssigning bool) tea.Cmd {
	if m.issue == nil {
		return nil
	}

	if !m.isAssigning && isAssigning {
		m.inputBox.Reset()
		m.ac.Reset() // Clear any stale autocomplete state (e.g., from labeling)
	}
	m.isAssigning = isAssigning
	m.inputBox.SetPrompt(constants.AssignPrompt)
	if !m.userAssignedToIssue(m.ctx.User) {
		m.inputBox.SetValue(m.ctx.User)
	}

	if isAssigning {
		return tea.Sequence(textarea.Blink, m.inputBox.Focus())
	}
	return nil
}

func (m *Model) SetIsLabeling(isLabeling bool) tea.Cmd {
	if m.issue == nil {
		return nil
	}

	if !m.isLabeling && isLabeling {
		m.inputBox.Reset()
	}
	m.isLabeling = isLabeling
	m.inputBox.SetPrompt(constants.LabelPrompt)

	labels := make([]string, 0)
	for _, label := range m.issue.Data.Labels.Nodes {
		labels = append(labels, label.Name)
	}
	labels = append(labels, "")
	m.inputBox.SetValue(strings.Join(labels, ", "))

	// Reset autocomplete
	m.ac.Hide()
	m.ac.SetSuggestions(nil)

	// Trigger label fetching for autocomplete
	if isLabeling {
		repoName := m.issue.Data.GetRepoNameWithOwner()
		if labels, ok := data.GetCachedRepoLabels(repoName); ok {
			// Use cached labels
			m.repoLabels = labels
			m.ac.SetSuggestions(data.GetLabelNames(labels))
			cursorPos := m.inputBox.GetCursorPosition()
			currentLabel := labelAtCursor(cursorPos, m.inputBox.Value())
			existingLabels := allLabels(m.inputBox.Value())
			m.ac.Show(currentLabel, existingLabels)
			return tea.Sequence(textarea.Blink, m.inputBox.Focus())
		} else {
			// Fetch labels asynchronously
			return tea.Sequence(m.fetchLabels(), textarea.Blink, m.inputBox.Focus())
		}
	}
	return nil
}

// fetchLabels returns a command to fetch repository labels
func (m *Model) fetchLabels() tea.Cmd {
	spinnerTickCmd := m.ac.SetFetchLoading()

	fetchCmd := func() tea.Msg {
		repoName := m.issue.Data.GetRepoNameWithOwner()
		labels, err := data.FetchRepoLabels(repoName)
		if err != nil {
			return RepoLabelsFetchFailedMsg{Err: err}
		}
		return RepoLabelsFetchedMsg{Labels: labels}
	}

	return tea.Batch(spinnerTickCmd, fetchCmd)
}

func (m *Model) userAssignedToIssue(login string) bool {
	for _, a := range m.issue.Data.Assignees.Nodes {
		if login == a.Login {
			return true
		}
	}
	return false
}

func (m *Model) GetIsUnassigning() bool {
	return m.isUnassigning
}

func (m *Model) SetIsUnassigning(isUnassigning bool) tea.Cmd {
	if m.issue == nil {
		return nil
	}

	if !m.isUnassigning && isUnassigning {
		m.inputBox.Reset()
		m.ac.Reset() // Clear any stale autocomplete state (e.g., from labeling)
	}
	m.isUnassigning = isUnassigning
	m.inputBox.SetPrompt(constants.UnassignPrompt)
	m.inputBox.SetValue(strings.Join(m.issueAssignees(), "\n"))

	if isUnassigning {
		return tea.Sequence(textarea.Blink, m.inputBox.Focus())
	}
	return nil
}

func (m *Model) issueAssignees() []string {
	var assignees []string
	for _, n := range m.issue.Data.Assignees.Nodes {
		assignees = append(assignees, n.Login)
	}
	return assignees
}

func (m *Model) UpdateProgramContext(ctx *context.ProgramContext) {
	m.ctx = ctx
	m.inputBox.UpdateProgramContext(ctx)
	m.ac.UpdateProgramContext(ctx)
}

func (m *Model) GetNumComments() int {
	if m.issue == nil {
		return 0
	}
	return len(m.issue.Data.Comments.Nodes)
}

func (m *Model) GetSelectedCommentIndex() int {
	return m.selectedCommentIndex
}

func (m *Model) SelectNextComment() {
	numComments := m.GetNumComments()
	if numComments == 0 {
		return
	}
	if m.selectedCommentIndex < numComments-1 {
		m.selectedCommentIndex++
	}
}

func (m *Model) SelectPrevComment() {
	if m.selectedCommentIndex > 0 {
		m.selectedCommentIndex--
	} else if m.selectedCommentIndex == -1 && m.GetNumComments() > 0 {
		m.selectedCommentIndex = 0
	}
}

func (m *Model) ResetCommentSelection() {
	m.selectedCommentIndex = -1
}

func (m *Model) GetSelectedComment() *data.IssueComment {
	if m.issue == nil || m.selectedCommentIndex < 0 {
		return nil
	}
	comments := m.issue.Data.Comments.Nodes
	if m.selectedCommentIndex >= len(comments) {
		return nil
	}

	// Sort comments by UpdatedAt to match rendering order
	type indexedComment struct {
		index   int
		comment data.IssueComment
	}
	sorted := make([]indexedComment, len(comments))
	for i, c := range comments {
		sorted[i] = indexedComment{index: i, comment: c}
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].comment.UpdatedAt.Before(sorted[j].comment.UpdatedAt)
	})

	return &sorted[m.selectedCommentIndex].comment
}

func (m *Model) SetIsQuoteReplying(comment *data.IssueComment) tea.Cmd {
	if m.issue == nil || comment == nil {
		return nil
	}

	m.inputBox.Reset()
	m.isCommenting = true
	m.expandInputBoxForCommenting()

	// Format the quoted comment
	var quotedLines []string
	quotedLines = append(quotedLines, fmt.Sprintf("> @%s wrote:", comment.Author.Login))
	quotedLines = append(quotedLines, ">")

	// Split comment body into lines and quote each
	bodyLines := strings.Split(comment.Body, "\n")
	for _, line := range bodyLines {
		quotedLines = append(quotedLines, "> "+line)
	}

	// Add empty line after quote for user's reply
	quotedLines = append(quotedLines, "")
	quotedLines = append(quotedLines, "")

	quotedText := strings.Join(quotedLines, "\n")
	m.inputBox.SetValue(quotedText)
	m.inputBox.SetPrompt("Reply to comment...")

	return tea.Sequence(textarea.Blink, m.inputBox.Focus())
}
