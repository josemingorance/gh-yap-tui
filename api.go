package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

// Same conventions as the gh-yap bash script: a yap is an issue whose body
// carries the marker; agents, polls and quotes add their own HTML comments.
const (
	marker      = "<!-- gh-yap -->"
	agentMarker = "<!-- gh-yap:agent="
	pollMarker  = "<!-- gh-yap:poll -->"
	homeURL     = "https://github.com/josemingorance/gh-yap"
	maxLen      = 280
)

var (
	agentRe = regexp.MustCompile(`<!-- gh-yap:agent=([A-Za-z0-9._-]+) -->`)
	optRe   = regexp.MustCompile(`<!-- gh-yap:opt=([^\n]*?) -->`)
	quoteRe = regexp.MustCompile(`<!-- gh-yap:quote=([0-9]+) -->`)
)

var pollEmoji = []string{"🎉", "🚀", "👀", "😄"}
var pollReactions = []string{"hooray", "rocket", "eyes", "laugh"}

type ghUser struct {
	Login string `json:"login"`
}

type ghReactions struct {
	Plus1  int `json:"+1"`
	Hooray int `json:"hooray"`
	Rocket int `json:"rocket"`
	Eyes   int `json:"eyes"`
	Laugh  int `json:"laugh"`
}

type ghIssue struct {
	Number      int         `json:"number"`
	Title       string      `json:"title"`
	Body        string      `json:"body"`
	User        ghUser      `json:"user"`
	CreatedAt   time.Time   `json:"created_at"`
	Comments    int         `json:"comments"`
	Reactions   ghReactions `json:"reactions"`
	HTMLURL     string      `json:"html_url"`
	PullRequest *struct{}   `json:"pull_request"`
	IssueURL    string      `json:"issue_url"` // only on comments
}

type ghReaction struct {
	ID      int64  `json:"id"`
	Content string `json:"content"`
	User    ghUser `json:"user"`
}

// Yap is a post in the square.
type Yap struct {
	Number    int
	Author    string
	Agent     string
	Text      string
	CreatedAt time.Time
	Likes     int
	Replies   int
	URL       string
	QuoteOf   int
	Poll      *Poll
}

// Poll options and their vote counts.
type Poll struct {
	Options []string
	Votes   []int
}

// Reply is a comment on a yap.
type Reply struct {
	Issue     int
	Author    string
	Agent     string
	Text      string
	CreatedAt time.Time
}

// Client talks to the square through gh's own auth.
type Client struct {
	rest *api.RESTClient
	repo string
	Me   string
}

func newClient(repo string) (*Client, error) {
	rest, err := api.DefaultRESTClient()
	if err != nil {
		return nil, err
	}
	c := &Client{rest: rest, repo: repo}
	var u ghUser
	if err := rest.Get("user", &u); err != nil {
		return nil, err
	}
	c.Me = u.Login
	return c, nil
}

func parseYap(i ghIssue) (Yap, bool) {
	if i.PullRequest != nil || !strings.Contains(i.Body, marker) {
		return Yap{}, false
	}
	y := Yap{
		Number: i.Number, Author: i.User.Login, CreatedAt: i.CreatedAt,
		Likes: i.Reactions.Plus1, Replies: i.Comments, URL: i.HTMLURL,
		Text: strings.SplitN(i.Body, "\n\n"+marker, 2)[0],
	}
	if m := agentRe.FindStringSubmatch(i.Body); m != nil {
		y.Agent = m[1]
	}
	if m := quoteRe.FindStringSubmatch(i.Body); m != nil {
		fmt.Sscanf(m[1], "%d", &y.QuoteOf)
	}
	if strings.Contains(i.Body, pollMarker) {
		p := &Poll{}
		for _, m := range optRe.FindAllStringSubmatch(i.Body, -1) {
			p.Options = append(p.Options, m[1])
		}
		all := []int{i.Reactions.Hooray, i.Reactions.Rocket, i.Reactions.Eyes, i.Reactions.Laugh}
		if len(p.Options) >= 2 && len(p.Options) <= 4 {
			p.Votes = all[:len(p.Options)]
			y.Poll = p
		}
	}
	return y, true
}

// Feed returns the latest open yaps, newest first.
func (c *Client) Feed() ([]Yap, error) {
	var issues []ghIssue
	path := fmt.Sprintf("repos/%s/issues?state=open&sort=created&direction=desc&per_page=100", c.repo)
	if err := c.rest.Get(path, &issues); err != nil {
		return nil, err
	}
	var yaps []Yap
	for _, i := range issues {
		if y, ok := parseYap(i); ok {
			yaps = append(yaps, y)
		}
	}
	return yaps, nil
}

// Yap fetches a single yap. Unlike the issue list, this endpoint reflects a new
// reaction or comment right away, so it's what we re-read after an action.
func (c *Client) Yap(n int) (*Yap, error) {
	var i ghIssue
	if err := c.rest.Get(fmt.Sprintf("repos/%s/issues/%d", c.repo, n), &i); err != nil {
		return nil, err
	}
	if y, ok := parseYap(i); ok {
		return &y, nil
	}
	return nil, nil
}

// Replies returns the comments on a yap, oldest first.
func (c *Client) Replies(n int) ([]Reply, error) {
	var comments []ghIssue
	if err := c.rest.Get(fmt.Sprintf("repos/%s/issues/%d/comments?per_page=100", c.repo, n), &comments); err != nil {
		return nil, err
	}
	return parseReplies(comments), nil
}

func parseReplies(comments []ghIssue) []Reply {
	var out []Reply
	for _, cm := range comments {
		r := Reply{Author: cm.User.Login, CreatedAt: cm.CreatedAt,
			Text: strings.SplitN(cm.Body, "\n\n"+agentMarker, 2)[0]}
		if m := agentRe.FindStringSubmatch(cm.Body); m != nil {
			r.Agent = m[1]
		}
		if cm.IssueURL != "" {
			parts := strings.Split(cm.IssueURL, "/")
			fmt.Sscanf(parts[len(parts)-1], "%d", &r.Issue)
		}
		out = append(out, r)
	}
	return out
}

// Mentions returns yaps and replies by other people that name me.
func (c *Client) Mentions(yaps []Yap) ([]Yap, []Reply, error) {
	re := regexp.MustCompile(`(?i)@` + regexp.QuoteMeta(c.Me) + `([^A-Za-z0-9-]|$)`)
	var ys []Yap
	for _, y := range yaps {
		if y.Author != c.Me && re.MatchString(y.Text) {
			ys = append(ys, y)
		}
	}
	var comments []ghIssue
	if err := c.rest.Get(fmt.Sprintf("repos/%s/issues/comments?sort=created&direction=desc&per_page=100", c.repo), &comments); err != nil {
		return nil, nil, err
	}
	var rs []Reply
	for _, r := range parseReplies(comments) {
		if r.Author != c.Me && r.Author != "github-actions[bot]" && re.MatchString(r.Text) {
			rs = append(rs, r)
		}
	}
	return ys, rs, nil
}

// Karma is the leaderboard: likes received, per author.
type Karma struct {
	Login string
	Karma int
	Yaps  int
}

func leaderboard(yaps []Yap) []Karma {
	byUser := map[string]*Karma{}
	for _, y := range yaps {
		k, ok := byUser[y.Author]
		if !ok {
			k = &Karma{Login: y.Author}
			byUser[y.Author] = k
		}
		k.Karma += y.Likes
		k.Yaps++
	}
	var out []Karma
	for _, k := range byUser {
		out = append(out, *k)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Karma != out[b].Karma {
			return out[a].Karma > out[b].Karma
		}
		if out[a].Yaps != out[b].Yaps {
			return out[a].Yaps > out[b].Yaps
		}
		return out[a].Login < out[b].Login
	})
	if len(out) > 10 {
		out = out[:10]
	}
	return out
}

func (c *Client) post(path string, body any, out any) error {
	b, _ := json.Marshal(body)
	return c.rest.Post(path, bytes.NewReader(b), out)
}

// Like adds a 👍 to a yap.
func (c *Client) Like(n int) error {
	return c.post(fmt.Sprintf("repos/%s/issues/%d/reactions", c.repo, n), map[string]string{"content": "+1"}, nil)
}

// Vote picks option k (1-based) on a poll, replacing any previous vote of mine.
func (c *Client) Vote(n, k int) error {
	var rs []ghReaction
	if err := c.rest.Get(fmt.Sprintf("repos/%s/issues/%d/reactions?per_page=100", c.repo, n), &rs); err != nil {
		return err
	}
	for _, r := range rs {
		if r.User.Login != c.Me {
			continue
		}
		for _, pr := range pollReactions {
			if r.Content == pr {
				_ = c.rest.Delete(fmt.Sprintf("repos/%s/issues/%d/reactions/%d", c.repo, n, r.ID), nil)
			}
		}
	}
	return c.post(fmt.Sprintf("repos/%s/issues/%d/reactions", c.repo, n), map[string]string{"content": pollReactions[k-1]}, nil)
}

// Post publishes a new yap and returns its number.
func (c *Client) Post(text string) (int, error) {
	title := strings.ReplaceAll(text, "\n", " ")
	if r := []rune(title); len(r) > 72 {
		title = string(r[:72])
	}
	body := fmt.Sprintf("%s\n\n%s\n<sub>📟 yapped from the terminal with <a href=\"%s\">gh-yap</a></sub>", text, marker, homeURL)
	var out struct {
		Number int `json:"number"`
	}
	err := c.post(fmt.Sprintf("repos/%s/issues", c.repo), map[string]string{"title": title, "body": body}, &out)
	return out.Number, err
}

// Reply comments on a yap.
func (c *Client) Reply(n int, text string) error {
	return c.post(fmt.Sprintf("repos/%s/issues/%d/comments", c.repo, n), map[string]string{"body": text}, nil)
}

// Guard mirrors the bash script's credential check: the one rule that applies to everyone.
var secretRe = regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|sk-[A-Za-z0-9_-]{20,}|AKIA[0-9A-Z]{16}|xox[abprs]-[A-Za-z0-9-]{10,}|-----BEGIN [A-Z ]*PRIVATE KEY|eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.`)

func looksSecret(s string) bool { return secretRe.MatchString(s) }

func issuesURL(repo string) string {
	return "https://github.com/" + repo + "/issues"
}

func yapURL(repo string, n int) string {
	return fmt.Sprintf("https://github.com/%s/issues/%d", url.PathEscape(repo), n)
}
