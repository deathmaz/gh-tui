package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

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

const (
	prs         = "prs"
	description = "description"
)

type repo struct {
	NameWithOwner string `json:"nameWithOwner"`
	Name          string `json:"name"`
	Owner         Owner  `json:"owner"`
}

var docStyle = lipgloss.NewStyle().Margin(1, 2)

type PullRequest struct {
	Url         string
	CreatedAt   string
	Title       string
	Number      int
	BaseRefName string
	Author      struct {
		Login string
	}
}

type ReviewRequest struct {
	RequestedReviewer struct {
		User struct {
			Login string
		} `graphql:"... on User"`
	}
}
type PullRequestDetails struct {
	Url         string
	Body        string
	State       string
	CreatedAt   string
	Title       string
	Number      int
	HeadRefName string
	BaseRefName string
	Author      struct {
		Login string
	}
	ReviewRequests struct {
		Nodes []ReviewRequest
	} `graphql:"reviewRequests(first:10)"`
}

type PullRequestDetailsRequest struct {
	Url         string
	Body        string
	State       string
	CreatedAt   string
	Title       string
	Number      int
	HeadRefName string
	BaseRefName string
	Author      struct {
		Login string
	}
	ReviewRequests []ReviewRequest `graphql:"reviewRequests(first:10)"`
}

type item struct {
	title, desc string
	Number      int
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

type app struct {
	list         list.Model
	prList       []PullRequest
	err          error
	repo         repo
	currentView  string
	prDetails    PullRequestDetails
	changedFiles string
}

type PrDescription struct {
	details      PullRequestDetails
	changedFiles string
}

type (
	listMsg []PullRequest
	prMsg   PrDescription
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

func (m app) getPulls() tea.Msg {
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

func (m app) getPull(number int) tea.Msg {
	client, err := api.DefaultGraphQLClient()
	if err != nil {
		return errMsg{err}
	}
	var query struct {
		Repository struct {
			PullRequest PullRequestDetails `graphql:"pullRequest(number: $number)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}
	variables := map[string]interface{}{
		"number": graphql.Int(number),
		"owner":  graphql.String(m.repo.Owner.Login),
		"name":   graphql.String(m.repo.Name),
	}
	err = client.Query("PullRequest", &query, variables)
	if err != nil {
		return errMsg{err}
	}
	args := []string{
		"pr",
		"diff",
		fmt.Sprintf("%d", number),
		"-R",
		m.repo.NameWithOwner,
		"--name-only",
	}
	stdOut, _, err := gh.Exec(args...)
	if err != nil {
		log.Fatal(err)
	}

	return prMsg(PrDescription{
		details:      query.Repository.PullRequest,
		changedFiles: stdOut.String(),
	})
}

func (m app) View() string {
	if m.currentView == prs {
		return docStyle.Render(m.list.View())
	} else if m.currentView == description {
		s := strings.Builder{}
		s.WriteString(m.prDetails.Title)
		s.WriteString("\n")
		s.WriteString(fmt.Sprintf(
			"%s wants to merge xx commits into %s from %s",
			m.prDetails.Author.Login,
			m.prDetails.BaseRefName,
			m.prDetails.HeadRefName,
		))
		s.WriteString("\n")

		body := "No description provided"
		if m.prDetails.Body != "" {
			body = m.prDetails.Body
		}
		s.WriteString(body)
		s.WriteString("\n")

		for _, r := range m.prDetails.ReviewRequests.Nodes {
			s.WriteString(fmt.Sprintf("%s", r.RequestedReviewer.User.Login))
			s.WriteString("\n")
		}

		s.WriteString(m.changedFiles)
		s.WriteString("\n")

		return docStyle.Render(s.String())
	}

	return "Loading..."
}

func (m app) Init() tea.Cmd {
	// return m.getViaRestClient
	return m.getPulls
}

func (m app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "d":
			selItem := m.list.SelectedItem().(item)
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
		case "i":
			return m, func() tea.Msg {
				item := m.list.SelectedItem().(item)
				return m.getPull(item.Number)
			}
		case "h":
			if m.currentView == description {
				m.currentView = prs
				return m, nil
			}
		}
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
	case prMsg:
		m.prDetails = PullRequestDetails(msg.details)
		m.changedFiles = msg.changedFiles
		m.currentView = description
	case listMsg:
		items := make([]list.Item, 0, len(msg))
		for _, p := range msg {
			date, _ := time.Parse(time.RFC3339, p.CreatedAt)
			items = append(items, item{
				title: fmt.Sprintf("%s", p.Title),
				desc: fmt.Sprintf(
					"#%d opened on %d-%02d-%02d %02d:%02d by %s",
					p.Number,
					date.Day(),
					date.Month(),
					date.Year(),
					date.Hour(),
					date.Minute(),
					p.Author.Login,
				),
				Number: p.Number,
			})
		}
		m.list.SetItems(items)
		m.prList = msg
	case errMsg:
		fmt.Printf("%s", msg)
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
		list:        list.New(items, list.NewDefaultDelegate(), 0, 0),
		repo:        repo,
		currentView: prs,
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
