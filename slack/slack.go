package slack

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nlopes/slack"
	"github.com/philopon/github-events-to-slack/github"
)

const githubBase = "https://github.com"

type Parsed github.Parsed

type Message struct {
	Messager
	Parsed
}

type Messager interface {
	Text(Parsed) string
	Attachments(Parsed) []slack.Attachment
}

func ParseEvent(e github.Event) (Message, error) {
	var err error
	var msg Messager

	switch e.Type {
	case "PushEvent":
		var payload PushEvent
		err = json.Unmarshal(e.Payload, &payload)
		msg = &payload
	case "IssueCommentEvent":
		var payload IssueCommentEvent
		err = json.Unmarshal(e.Payload, &payload)
		msg = &payload
	case "IssuesEvent":
		var payload IssuesEvent
		err = json.Unmarshal(e.Payload, &payload)
		msg = &payload
	case "PullRequestEvent":
		var payload PullRequestEvent
		err = json.Unmarshal(e.Payload, &payload)
		msg = &payload
	case "PullRequestReviewCommentEvent":
		var payload PullRequestReviewCommentEvent
		err = json.Unmarshal(e.Payload, &payload)
		msg = &payload
	default:
		err = fmt.Errorf("unknown event: %v", e.Type)
	}

	return Message{msg, Parsed(e.Parsed)}, err
}

func SlackLink(url string, title string) string {
	return fmt.Sprintf("<%v|%v>", url, title)
}

func (i *Parsed) RepoName() string {
	return i.Repo.Name
}

func (i *Parsed) RepoURL() string {
	return fmt.Sprintf("%v/%v", githubBase, i.Repo.Name)
}

func (i *Parsed) RepoLink() string {
	return SlackLink(i.RepoURL(), i.RepoName())
}

func (i *Parsed) UserName() string {
	return i.Actor.Login
}

func (i *Parsed) UserURL() string {
	return fmt.Sprintf("%v/%v", githubBase, i.UserName())
}

func (i *Parsed) UserLink() string {
	return SlackLink(i.UserURL(), i.UserName())
}

func (i *Parsed) TreeURL(path string) string {
	return fmt.Sprintf("%v/tree/%v", i.RepoURL(), path)
}

func (i *Parsed) TreeLink(path string) string {
	return SlackLink(i.TreeURL(path), path)
}

type Event struct{}

func (e *Event) Text(parsed Parsed) string {
	return ""
}

func (e *Event) Attachments(parsed Parsed) []slack.Attachment {
	return []slack.Attachment{}
}

type PushEvent struct {
	Event
	Ref     string
	Commits []struct {
		Sha     string
		Message string
	}
}

func (p *PushEvent) Text(parsed Parsed) string {
	refs := strings.Split(p.Ref, "/")
	ref := refs[len(refs)-1]

	return fmt.Sprintf("*%v pushed to %v at %v*", parsed.UserLink(), parsed.TreeLink(ref), parsed.RepoLink())
}

func (p *PushEvent) Attachments(parsed Parsed) []slack.Attachment {
	commits := make([]string, 0, 5)
	for _, commit := range p.Commits {
		url := fmt.Sprintf("%v/%v/commit/%v", githubBase, parsed.RepoName(), commit.Sha)
		msg := strings.Split(commit.Message, "\n")[0]
		txt := fmt.Sprintf("%v %v", SlackLink(url, commit.Sha[:7]), msg)

		commits = append(commits, txt)
	}

	a := slack.Attachment{Text: strings.Join(commits, "\n"), MarkdownIn: []string{"text"}}
	return []slack.Attachment{a}
}

type IssueCommentEvent struct {
	Event
	Comment struct {
		HtmlUrl string `json:"html_url"`
		Body    string
	}
	Issue struct {
		Number int
	}
}

func (e *IssueCommentEvent) Text(p Parsed) string {
	issueName := fmt.Sprintf("%v#%v", p.RepoName(), e.Issue.Number)
	issueLink := SlackLink(e.Comment.HtmlUrl, issueName)

	body := e.Comment.Body
	lines := strings.Split(body, "\n")
	if len(lines) >= 3 {
		lines = lines[:3]
	}
	body = strings.Join(lines, "\n")

	return fmt.Sprintf("*%v commented on pull request %v*\n%v", p.UserLink(), issueLink, body)
}

type IssuesEvent struct {
	Event
	Action string
	Issue  struct {
		Title   string
		HtmlUrl string `json:"html_url"`
		Number  int
	}
}

func (e *IssuesEvent) Text(p Parsed) string {
	title := fmt.Sprintf("%v#%v", p.RepoName(), e.Issue.Number)
	link := SlackLink(e.Issue.HtmlUrl, title)
	return fmt.Sprintf("*%v %v issue %v*\n%v*", p.UserLink(), e.Action, link, e.Issue.Title)
}

type PullRequestEvent struct {
	Event
	Action      string
	Number      int
	PullRequest struct {
		Title   string
		HtmlUrl string `json:"html_url"`

		Commits   int
		Additions int
		Deletions int
	} `json:"pull_request"`
}

func (e *PullRequestEvent) Text(p Parsed) string {
	title := fmt.Sprintf("%v#%v", p.RepoName(), e.Number)
	link := SlackLink(e.PullRequest.HtmlUrl, title)
	return fmt.Sprintf("*%v %v pull request %v*\n%v", p.UserLink(), e.Action, link, e.PullRequest.Title)
}

func (e *PullRequestEvent) Attachments(p Parsed) []slack.Attachment {
	comT, addT, delT := "commits", "additions", "deletions"
	comN, addN, delN := e.PullRequest.Commits, e.PullRequest.Additions, e.PullRequest.Deletions

	if comN == 1 {
		comT = "commit"
	}
	if addN == 1 {
		addT = "addition"
	}
	if delN == 1 {
		delT = "deletion"
	}

	text := fmt.Sprintf("*%v* %v with *%v* %v and *%v* %v", comN, comT, addN, addT, delN, delT)
	a := slack.Attachment{Text: text, MarkdownIn: []string{"text"}}
	return []slack.Attachment{a}
}

type PullRequestReviewCommentEvent struct {
	Event
	Comment struct {
		HtmlUrl string `json:"html_url"`
		Body    string
	}
	PullRequest struct {
		Number int
	} `json:"pull_request"`
}

func (e PullRequestReviewCommentEvent) Text(p Parsed) string {
	title := fmt.Sprintf("%v#%v", p.RepoName(), e.PullRequest.Number)
	link := SlackLink(e.Comment.HtmlUrl, title)

	return fmt.Sprintf("*%v commented on pull request %v*\n%v", p.UserLink(), link, e.Comment.Body)
}

type client struct {
	*slack.Client
	channel string
}

func NewSlack(token string, channel string) *client {
	cli := slack.New(token)
	return &client{cli, channel}
}

func (sl *client) PostMessage(msg Message) error {
	prm := slack.NewPostMessageParameters()

	prm.Username = msg.UserName() + "[github event]"
	prm.Attachments = msg.Messager.Attachments(msg.Parsed)
	prm.UnfurlLinks = false
	prm.UnfurlMedia = false
	prm.IconURL = msg.Actor.AvatarUrl
	prm.Markdown = true
	prm.EscapeText = false

	_, _, err := sl.Client.PostMessage(sl.channel, msg.Text(msg.Parsed), prm)
	return err
}

func (sl *client) UploadEvent(event github.Event, msg string) error {
	cnt, err := json.MarshalIndent(&event.Raw, "", "\t")
	if err != nil {
		return err
	}

	prm := slack.FileUploadParameters{Filetype: "javascript", Content: string(cnt), Filename: event.Type + ".json", InitialComment: msg, Channels: []string{sl.channel}}

	_, err = sl.UploadFile(prm)
	return err
}
