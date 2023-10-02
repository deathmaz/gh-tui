package pr

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/cli/go-gh/pkg/text"
)

type Author struct {
	Login string
	Name  string
}

type CommitAuthor struct {
	Login string `json:"login"`
	Name  string `json:"name"`
}

type Commit struct {
	MessageBody     string         `json:"messageBody"`
	MessageHeadline string         `json:"messageHeadline"`
	CommitedDate    time.Time      `json:"committedDate"`
	Authors         []CommitAuthor `json:"authors"`
}

type ReviewRequest struct {
	Typename string `json:"__typename"`
	Login    string `json:"login"`
}

type File struct {
	Path      string `json:"path"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

type Details struct {
	Author         Author          `json:"author"`
	Title          string          `json:"title"`
	Url            string          `json:"url"`
	Number         int             `json:"number"`
	BaseRefName    string          `json:"baseRefName"`
	HeadRefName    string          `json:"headRefName"`
	Body           string          `json:"body"`
	Commits        []Commit        `json:"commits"`
	ReviewRequests []ReviewRequest `json:"reviewRequests"`
	Files          []File          `json:"files"`
	CreatedAt      time.Time       `json:"createdAt"`
}

func (d Details) Render() string {
	style := lipgloss.NewStyle()
	s := strings.Builder{}
	s.WriteString(
		style.MarginBottom(1).Bold(true).Foreground(lipgloss.Color("255")).Render(d.Title),
	)
	s.WriteString("\n")
	s.WriteString(fmt.Sprintf(
		"%s wants to merge xx commits into %s from %s • %s\n",
		style.Foreground(lipgloss.Color("255")).Bold(true).Render(d.Author.Login),
		style.Foreground(lipgloss.Color("39")).Render(d.BaseRefName),
		style.Foreground(lipgloss.Color("39")).Render(d.HeadRefName),
		FormatCreatedAt(d.CreatedAt),
	))
	s.WriteString("\n\n")

	body := "No description provided"
	if d.Body != "" {
		body = d.Body
	}
	s.WriteString(style.Bold(true).Render("Description:"))
	s.WriteString("\n")
	s.WriteString(style.MarginBottom(1).Foreground(lipgloss.Color("247")).Render(body))
	s.WriteString("\n")

	s.WriteString(style.Bold(true).Render("Requested reviewers:"))
	s.WriteString("\n")
	for _, r := range d.ReviewRequests {
		s.WriteString(
			style.Foreground(lipgloss.Color("247")).Bold(true).Render(fmt.Sprintf("%s", r.Login)),
		)
		s.WriteString("\n")
	}

	s.WriteString("\n")
	s.WriteString(style.Bold(true).Render("Commits:"))
	s.WriteString("\n")

	for _, commit := range d.Commits {
		s.WriteString("• ")
		s.WriteString(style.Foreground(lipgloss.Color("247")).Render(commit.MessageHeadline))
		s.WriteString(" by ")
		for _, author := range commit.Authors {
			if author.Login != "" {
				s.WriteString(author.Login)
			} else {
				s.WriteString(author.Name)
			}
		}
		s.WriteString("\n")
	}

	s.WriteString("\n")

	s.WriteString(style.Bold(true).Render("Changed files:"))
	s.WriteString("\n")

	f := strings.Builder{}
	for _, file := range d.Files {
		f.WriteString(style.Foreground(lipgloss.Color("247")).Render(file.Path))
		if file.Additions > 0 {
			f.WriteString(
				style.Foreground(lipgloss.Color("40")).
					Render(fmt.Sprintf(" +%d ", file.Additions)),
			)
		}
		if file.Deletions > 0 {
			f.WriteString(
				style.Foreground(lipgloss.Color("196")).
					Render(fmt.Sprintf(" -%d ", file.Deletions)),
			)
		}
		f.WriteString("\n")
	}
	s.WriteString(f.String())

	s.WriteString("\n")

	return s.String()
}

func FormatCreatedAt(d time.Time) string {
	ago := time.Now().Sub(d)

	if ago < 3*24*time.Hour {
		return text.RelativeTimeAgo(time.Now(), d)
	} else {
		return fmt.Sprintf("%s", d.Format("02-01-2006"))
	}
}
