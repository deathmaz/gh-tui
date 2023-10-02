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
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	gh "github.com/cli/go-gh"
	"github.com/cli/go-gh/pkg/browser"
	"github.com/cli/go-gh/v2/pkg/api"
	graphql "github.com/cli/shurcooL-graphql"
	"github.com/deathmaz/gh-tui/pr"
)

var (
	titleStyle = func() lipgloss.Style {
		b := lipgloss.RoundedBorder()
		b.Right = "├"
		return lipgloss.NewStyle().BorderStyle(b).Padding(0, 1)
	}()

	infoStyle = func() lipgloss.Style {
		b := lipgloss.RoundedBorder()
		b.Left = "┤"
		return titleStyle.Copy().BorderStyle(b)
	}()
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
	CreatedAt   time.Time
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

type item struct {
	title, desc string
	Number      int
	Url         string
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

type app struct {
	list        list.Model
	err         error
	repo        repo
	currentView string
	// TODO: remove ?
	PullDetails  PullRequestDetails
	changedFiles string
	viewport     viewport.Model
	ready        bool
}

func (m app) headerView() string {
	title := titleStyle.Render("Pr details")
	line := strings.Repeat("─", max(0, m.viewport.Width-lipgloss.Width(title)))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, line)
}

func (m app) footerView() string {
	info := infoStyle.Render(fmt.Sprintf("%3.f%%", m.viewport.ScrollPercent()*100))
	line := strings.Repeat("─", max(0, m.viewport.Width-lipgloss.Width(info)))
	return lipgloss.JoinHorizontal(lipgloss.Center, line, info)
}

type PrDescription struct {
	details      PullRequestDetails
	changedFiles string
}

type (
	listMsg      []PullRequest
	prMsg        PrDescription
	prDetailsMsg pr.Details
	errMsg       struct{ err error }
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

func (m app) getPrDetails(number int) tea.Msg {
	args := []string{
		"pr",
		"view",
		fmt.Sprintf("%d", number),
		"-R",
		m.repo.NameWithOwner,
		"--json",
		"reviewDecision,reviewRequests,reviews,statusCheckRollup,title,url,number,author,baseRefName,body,changedFiles,files,commits,headRefName,createdAt",
	}
	stdOut, _, err := gh.Exec(args...)
	if err != nil {
		return errMsg{err: err}
	}
	prDetails := pr.Details{}
	json.Unmarshal([]byte(stdOut.String()), &prDetails)
	return prDetailsMsg(prDetails)
}

func (m app) View() string {
	if m.currentView == prs {
		return docStyle.Render(m.list.View())
	} else if m.currentView == description {
		return fmt.Sprintf("%s\n%s\n%s", m.headerView(), m.viewport.View(), m.footerView())
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
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
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
				return m.getPrDetails(item.Number)
				// return m.getPull(item.Number)
			}
		case "o":
			b := browser.New("", os.Stdout, os.Stdin)
			item := m.list.SelectedItem().(item)
			err := b.Browse(item.Url)
			if err != nil {
				log.Fatal(err)
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

		headerHeight := lipgloss.Height(m.headerView())
		footerHeight := lipgloss.Height(m.footerView())
		verticalMarginHeight := headerHeight + footerHeight
		if !m.ready {
			// Since this program is using the full size of the viewport we
			// need to wait until we've received the window dimensions before
			// we can initialize the viewport. The initial dimensions come in
			// quickly, though asynchronously, which is why we wait for them
			// here.
			m.viewport = viewport.New(msg.Width, msg.Height-verticalMarginHeight)
			m.viewport.YPosition = headerHeight
			m.ready = true
			// Render the viewport one line below the header.
			m.viewport.YPosition = headerHeight + 1
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - verticalMarginHeight
		}
	case prDetailsMsg:
		m.currentView = description
		m.viewport.SetContent(lipgloss.NewStyle().Padding(0, 1).Render(pr.Details(msg).Render()))

	case prMsg:
		m.PullDetails = PullRequestDetails(msg.details)
		m.changedFiles = msg.changedFiles
		m.currentView = description

		s := strings.Builder{}
		s.WriteString(m.PullDetails.Title)
		s.WriteString("\n")
		s.WriteString(fmt.Sprintf(
			"%s wants to merge xx commits into %s from %s",
			m.PullDetails.Author.Login,
			m.PullDetails.BaseRefName,
			m.PullDetails.HeadRefName,
		))
		s.WriteString("\n")

		body := "No description provided"
		if m.PullDetails.Body != "" {
			body = m.PullDetails.Body
		}
		s.WriteString(body)
		s.WriteString("\n")

		for _, r := range m.PullDetails.ReviewRequests.Nodes {
			s.WriteString(fmt.Sprintf("%s", r.RequestedReviewer.User.Login))
			s.WriteString("\n")
		}

		s.WriteString(m.changedFiles)
		s.WriteString("\n")
		m.viewport.SetContent(s.String())

	case listMsg:
		items := make([]list.Item, 0, len(msg))
		for _, p := range msg {
			items = append(items, item{
				title: fmt.Sprintf("%s", p.Title),
				desc: fmt.Sprintf(
					"#%d opened %s by %s",
					p.Number,
					pr.FormatCreatedAt(p.CreatedAt),
					p.Author.Login,
				),
				Number: p.Number,
				Url:    p.Url,
			})
		}
		m.list.SetItems(items)
	case errMsg:
		fmt.Printf("%s", msg)
		m.err = msg
		return m, nil
	}
	var cmd tea.Cmd
	if m.currentView == prs {
		m.list, cmd = m.list.Update(msg)
	} else if m.currentView == description {
		m.viewport, cmd = m.viewport.Update(msg)
	}
	return m, cmd
}

func main() {
	repo, err := getRepoNameWithOwner()
	if err != nil {
		log.Fatal(err)
	}

	var items []list.Item
	d := list.NewDefaultDelegate()
	d.Styles.SelectedTitle.BorderForeground(lipgloss.Color("#494d65"))
	d.Styles.SelectedDesc.BorderForeground(lipgloss.Color("#494d65"))
	d.Styles.SelectedTitle.Foreground(lipgloss.Color("255"))
	d.Styles.SelectedDesc.Foreground(lipgloss.Color("247"))
	d.Styles.SelectedTitle.Background(lipgloss.Color("#494d65"))
	d.Styles.SelectedDesc.Background(lipgloss.Color("#494d65"))
	list := list.New(items, d, 0, 0)
	list.Title = "The list of pull requests:"

	a := app{
		list:        list,
		repo:        repo,
		currentView: prs,
	}
	p := tea.NewProgram(a, tea.WithAltScreen(), tea.WithMouseCellMotion())
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
