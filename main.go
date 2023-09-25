package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	gh "github.com/cli/go-gh"
	"github.com/cli/go-gh/v2/pkg/api"
)

type repo struct {
	NameWithOwner string `json:"nameWithOwner"`
}

var docStyle = lipgloss.NewStyle().Margin(1, 2)

type PullRequest struct {
	Url    string
	Title  string
	Number int
	State  string
}

type item struct {
	title, desc string
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

type app struct {
	list              list.Model
	prList            []PullRequest
	err               error
	cursor            int
	repoNameWithOwner string
}

type (
	listMsg []PullRequest
	errMsg  struct{ err error }
)

func (e errMsg) Error() string { return e.err.Error() }

func (m app) getViaRestClient() tea.Msg {
	var response []PullRequest
	client, err := api.DefaultRESTClient()
	if err != nil {
		return errMsg{err}
	}
	err = client.Get(fmt.Sprintf("repos/%s/pulls", m.repoNameWithOwner), &response)
	if err != nil {
		return errMsg{err}
	}
	return listMsg(response)
}

func (m app) View() string {
	if len(m.list.Items()) > 0 {
		return docStyle.Render(m.list.View())
	}
	s := strings.Builder{}
	for i, p := range m.prList {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		s.WriteString(fmt.Sprintf("%s  %s", cursor, p.Title))
		s.WriteString("\n")
	}
	s.WriteString("\n")

	return s.String()
}

func (m app) Init() tea.Cmd {
	return m.getViaRestClient
}

func (m app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
	case listMsg:
		var items []list.Item
		for _, p := range msg {
			items = append(items, item{
				title: p.Title,
				desc:  p.State,
			})
		}
		m.list.SetItems(items)
		m.prList = msg
	case errMsg:
		m.err = msg
		return m, nil
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func main() {
	repoNameWithOwner, err := getRepoNameWithOwner()
	if err != nil {
		log.Fatal(err)
	}
	var items []list.Item
	a := app{
		list:              list.New(items, list.NewDefaultDelegate(), 0, 0),
		repoNameWithOwner: repoNameWithOwner,
	}
	p := tea.NewProgram(a, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("error: %v", err)
		os.Exit(1)
	}
}

func getRepoNameWithOwner() (string, error) {
	args := []string{"repo", "view", "--json", "nameWithOwner"}
	stdOut, _, err := gh.Exec(args...)
	if err != nil {
		return "", err
	}
	res := repo{}
	json.Unmarshal([]byte(stdOut.String()), &res)
	return res.NameWithOwner, nil
}
