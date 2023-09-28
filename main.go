package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	gh "github.com/cli/go-gh"
	"github.com/cli/go-gh/v2/pkg/api"
	graphql "github.com/cli/shurcooL-graphql"
)

type Owner struct {
	Id    string
	Login string
}

type repo struct {
	NameWithOwner string `json:"nameWithOwner"`
	Name          string `json:"name"`
	Owner         Owner  `json:"owner"`
}

var docStyle = lipgloss.NewStyle().Margin(1, 2)

type PullRequest struct {
	Url    string
	Title  string
	Number int
	State  string
	Author struct {
		Login string
	}
}

type item struct {
	title, desc string
	Number      int
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

type app struct {
	list   list.Model
	prList []PullRequest
	err    error
	cursor int
	repo   repo
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
	err = client.Get(fmt.Sprintf("repos/%s/pulls", m.repo.NameWithOwner), &response)
	if err != nil {
		return errMsg{err}
	}
	return listMsg(response)
}

func (m app) getViaGraphQL() tea.Msg {
	client, err := api.DefaultGraphQLClient()
	if err != nil {
		return errMsg{err}
	}
	var query struct {
		Repository struct {
			PullRequests struct {
				Nodes []PullRequest
			} `graphql:"pullRequests(first:20, states:OPEN, orderBy: { field: CREATED_AT, direction: DESC })"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}
	variables := map[string]interface{}{
		"owner": graphql.String(m.repo.Owner.Login),
		"name":  graphql.String(m.repo.Name),
	}
	err = client.Query("PullRequests", &query, variables)
	if err != nil {
		return errMsg{err}
	}

	return listMsg(query.Repository.PullRequests.Nodes)
}

func (m app) View() string {
	return docStyle.Render(m.list.View())
}

func (m app) Init() tea.Cmd {
	// return m.getViaRestClient
	return m.getViaGraphQL
}

func (m app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "d":
			selItem := m.list.SelectedItem().(item)
			fmt.Printf("%d", selItem.Number)
			c := exec.Command(
				"gh",
				"pr",
				"diff",
				fmt.Sprintf("%d", selItem.Number),
				"-R",
				m.repo.NameWithOwner,
			)
			cmd := tea.ExecProcess(c, func(err error) tea.Msg {
				return errMsg{err: err}
			})
			return m, cmd
		case "v":
			selItem := m.list.SelectedItem().(item)
			fmt.Printf("%d", selItem.Number)
			c := exec.Command(
				"gh",
				"pr",
				"view",
				fmt.Sprintf("%d", selItem.Number),
				"-R",
				m.repo.NameWithOwner,
			)
			cmd := tea.ExecProcess(c, func(err error) tea.Msg {
				return errMsg{err: err}
			})
			return m, cmd
		}
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
	case listMsg:
		items := make([]list.Item, 0, len(msg))
		for _, p := range msg {
			items = append(items, item{
				title:  p.Title,
				desc:   fmt.Sprintf("%s %d", p.Author.Login, p.Number),
				Number: p.Number,
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
	repo, err := getRepoNameWithOwner()
	if err != nil {
		log.Fatal(err)
	}
	var items []list.Item
	a := app{
		list: list.New(items, list.NewDefaultDelegate(), 0, 0),
		repo: repo,
	}
	p := tea.NewProgram(a, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("error: %v", err)
		os.Exit(1)
	}
}

func getRepoNameWithOwner() (repo, error) {
	args := []string{"repo", "view", "--json", "nameWithOwner,name,owner"}
	stdOut, _, err := gh.Exec(args...)
	if err != nil {
		return repo{}, err
	}
	res := repo{}
	json.Unmarshal([]byte(stdOut.String()), &res)
	return res, nil
}
