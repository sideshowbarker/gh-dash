package tui

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/issuessection"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/notificationssection"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/tasks"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/constants"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
)

type editorCommentFinishedMsg struct {
	Body        string
	Number      int
	RepoName    string
	IsPR        bool
	SectionId   int
	SectionType string
}

func getEditorCmd() string {
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}
	if visual := os.Getenv("VISUAL"); visual != "" {
		return visual
	}
	return "vi"
}

func (m *Model) openEditorComment(isPR bool) tea.Cmd {
	var number int
	var repoName string
	var sectionId int
	var sectionType string

	if m.ctx.View == config.NotificationsView {
		if isPR {
			pr := m.notificationView.GetSubjectPR()
			if pr == nil {
				return nil
			}
			number = pr.GetNumber()
			repoName = pr.GetRepoNameWithOwner()
		} else {
			issue := m.notificationView.GetSubjectIssue()
			if issue == nil {
				return nil
			}
			number = issue.GetNumber()
			repoName = issue.GetRepoNameWithOwner()
		}
		sectionId = 0
		sectionType = notificationssection.SectionType
	} else {
		currRowData := m.getCurrRowData()
		if currRowData == nil {
			return nil
		}
		number = currRowData.GetNumber()
		repoName = currRowData.GetRepoNameWithOwner()
		currSection := m.getCurrSection()
		if currSection != nil {
			sectionId = currSection.GetId()
			sectionType = currSection.GetType()
		}
	}

	tmpFile, err := os.CreateTemp("", "gh-dash-comment-*.md")
	if err != nil {
		return func() tea.Msg {
			return constants.ErrMsg{Err: fmt.Errorf("failed to create temp file: %w", err)}
		}
	}

	kind := "PR"
	if !isPR {
		kind = "issue"
	}
	header := fmt.Sprintf(
		"# Comment on %s #%d in %s\n"+
			"# Lines starting with '#' will be ignored.\n"+
			"# Save and quit to submit. Leave empty to cancel.\n",
		kind, number, repoName,
	)
	_, _ = fmt.Fprint(tmpFile, header)
	tmpFile.Close()

	editor := getEditorCmd()
	tmpPath := tmpFile.Name()

	c := exec.Command(editor, tmpPath)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		defer os.Remove(tmpPath)

		if err != nil {
			return constants.ErrMsg{Err: fmt.Errorf("editor exited with error: %w", err)}
		}

		f, err := os.Open(tmpPath)
		if err != nil {
			return constants.ErrMsg{Err: fmt.Errorf("failed to read temp file: %w", err)}
		}
		defer f.Close()

		var lines []string
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "#") {
				lines = append(lines, line)
			}
		}

		body := strings.TrimSpace(strings.Join(lines, "\n"))

		return editorCommentFinishedMsg{
			Body:        body,
			Number:      number,
			RepoName:    repoName,
			IsPR:        isPR,
			SectionId:   sectionId,
			SectionType: sectionType,
		}
	})
}

func (m *Model) submitEditorComment(msg editorCommentFinishedMsg) tea.Cmd {
	kind := "PR"
	ghCmd := "pr"
	if !msg.IsPR {
		kind = "issue"
		ghCmd = "issue"
	}

	taskId := fmt.Sprintf("editor_%s_comment_%d", ghCmd, msg.Number)
	task := context.Task{
		Id:           taskId,
		StartText:    fmt.Sprintf("Commenting on %s #%d", kind, msg.Number),
		FinishedText: fmt.Sprintf("Commented on %s #%d", kind, msg.Number),
		State:        context.TaskStart,
		Error:        nil,
	}
	startCmd := m.ctx.StartTask(task)

	return tea.Batch(startCmd, func() tea.Msg {
		c := exec.Command(
			"gh",
			ghCmd,
			"comment",
			fmt.Sprint(msg.Number),
			"-R",
			msg.RepoName,
			"-b",
			msg.Body,
		)

		err := c.Run()

		var innerMsg tea.Msg
		if msg.IsPR {
			innerMsg = tasks.UpdatePRMsg{
				PrNumber: msg.Number,
				NewComment: &data.Comment{
					Author:    struct{ Login string }{Login: m.ctx.User},
					Body:      msg.Body,
					UpdatedAt: time.Now(),
				},
			}
		} else {
			innerMsg = issuessection.UpdateIssueMsg{
				IssueNumber: msg.Number,
				NewComment: &data.IssueComment{
					Author:    struct{ Login string }{Login: m.ctx.User},
					Body:      msg.Body,
					UpdatedAt: time.Now(),
				},
			}
		}

		return constants.TaskFinishedMsg{
			SectionId:   msg.SectionId,
			SectionType: msg.SectionType,
			TaskId:      taskId,
			Err:         err,
			Msg:         innerMsg,
		}
	})
}
