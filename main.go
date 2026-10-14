// gh-yap-tui — the full-screen terminal interface for gh-yap.
//
//	gh yap tui          (or: gh yap-tui)
//
// It reads and writes the same square as the gh-yap script, through gh's auth.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const version = "0.1.1"
const refreshEvery = 30 * time.Second

// ---------------------------------------------------------------------------
// styles
// ---------------------------------------------------------------------------

var (
	cDim     = lipgloss.Color("8")
	cUser    = lipgloss.Color("3")
	cAgent   = lipgloss.Color("6")
	cHeart   = lipgloss.Color("1")
	cTag     = lipgloss.Color("4")
	cMention = lipgloss.Color("5")
	cOK      = lipgloss.Color("2")
	cBar     = lipgloss.Color("5")

	sTitle   = lipgloss.NewStyle().Bold(true).Foreground(cMention)
	sDim     = lipgloss.NewStyle().Foreground(cDim)
	sUser    = lipgloss.NewStyle().Bold(true).Foreground(cUser)
	sAgent   = lipgloss.NewStyle().Foreground(cAgent)
	sHeart   = lipgloss.NewStyle().Foreground(cHeart)
	sOK      = lipgloss.NewStyle().Foreground(cOK)
	sErr     = lipgloss.NewStyle().Foreground(cHeart)
	sBar     = lipgloss.NewStyle().Foreground(cBar)
	sTabOn   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(cMention).Padding(0, 1)
	sTabOff  = lipgloss.NewStyle().Foreground(cDim).Padding(0, 1)
	sCursor  = lipgloss.NewStyle().Foreground(cMention).Bold(true)
	sFrame   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cDim)
	sCompose = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cMention).Padding(0, 1)
)

var (
	mentionRe = regexp.MustCompile(`(^|[^\w])(@[A-Za-z0-9-]+)`)
	tagRe     = regexp.MustCompile(`(^|[^\w&])(#[A-Za-z][A-Za-z0-9_]*)`)
)

// ---------------------------------------------------------------------------
// model
// ---------------------------------------------------------------------------

type tab int

const (
	tabFeed tab = iota
	tabPolls
	tabMentions
	tabTop
	tabAgents
	tabCount
)

type mode int

const (
	modeList mode = iota
	modeDetail
	modeCompose
)

type model struct {
	client *Client
	repo   string

	width, height int
	tab           tab
	mode          mode
	cursor        int
	scroll        int // first visible line of the list

	yaps     []Yap
	mentions []Yap
	mentionR []Reply
	loading  bool
	err      error
	status   string
	statusOK bool

	detail   *Yap
	replies  []Reply
	viewport viewport.Model

	composer    textarea.Model
	composeFor  int // 0 = new yap, else reply to #n
	lastRefresh time.Time
}

type feedMsg struct {
	yaps     []Yap
	mentions []Yap
	replies  []Reply
	err      error
}
type repliesMsg struct {
	n       int
	replies []Reply
	err     error
}
type actionMsg struct {
	text  string
	err   error
	yap   *Yap // the yap as it is right after the action
	isNew bool // a yap we just posted
}

// fresh re-reads yap n after an action and wraps it in an actionMsg.
func fresh(c *Client, n int, text string, isNew bool) tea.Msg {
	y, err := c.Yap(n)
	if err != nil {
		return actionMsg{text: text}
	}
	return actionMsg{text: text, yap: y, isNew: isNew}
}

type tickMsg time.Time

func newModel(c *Client, repo string) model {
	ta := textarea.New()
	ta.Placeholder = "…"
	ta.CharLimit = maxLen
	ta.ShowLineNumbers = false
	ta.SetHeight(3)
	ta.Prompt = ""
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	return model{client: c, repo: repo, loading: true, composer: ta, viewport: viewport.New(80, 20)}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.load(), tick())
}

func tick() tea.Cmd {
	return tea.Tick(refreshEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) load() tea.Cmd {
	c := m.client
	return func() tea.Msg {
		yaps, err := c.Feed()
		if err != nil {
			return feedMsg{err: err}
		}
		my, mr, err := c.Mentions(yaps)
		if err != nil {
			return feedMsg{yaps: yaps, err: err}
		}
		return feedMsg{yaps: yaps, mentions: my, replies: mr}
	}
}

func (m model) loadReplies(n int) tea.Cmd {
	c := m.client
	return func() tea.Msg {
		rs, err := c.Replies(n)
		return repliesMsg{n: n, replies: rs, err: err}
	}
}

// ---------------------------------------------------------------------------
// update
// ---------------------------------------------------------------------------

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.viewport.Width = m.width - 4
		m.viewport.Height = m.height - 8
		m.composer.SetWidth(m.width - 8)
		if m.detail != nil {
			m.viewport.SetContent(m.renderDetail())
		}
		return m, nil

	case tickMsg:
		if m.mode != modeCompose {
			return m, tea.Batch(m.load(), tick())
		}
		return m, tick()

	case feedMsg:
		m.loading = false
		m.err = msg.err
		if msg.yaps != nil {
			m.yaps = msg.yaps
			m.mentions, m.mentionR = msg.mentions, msg.replies
			m.lastRefresh = time.Now()
			if m.detail != nil {
				for i := range m.yaps {
					if m.yaps[i].Number == m.detail.Number {
						y := m.yaps[i]
						m.detail = &y
						m.viewport.SetContent(m.renderDetail())
					}
				}
			}
		}
		m.clampCursor()
		return m, nil

	case repliesMsg:
		if m.detail != nil && m.detail.Number == msg.n {
			m.replies = msg.replies
			if msg.err != nil {
				m.err = msg.err
			}
			m.viewport.SetContent(m.renderDetail())
		}
		return m, nil

	case actionMsg:
		if msg.err != nil {
			m.status, m.statusOK = fmt.Sprintf(t("error"), msg.err), false
			return m, nil
		}
		m.status, m.statusOK = msg.text, true
		// Apply the fresh copy instead of reloading the whole list: the list
		// endpoint lags a few seconds and would show the old counts.
		if y := msg.yap; y != nil {
			found := false
			for i := range m.yaps {
				if m.yaps[i].Number == y.Number {
					m.yaps[i], found = *y, true
				}
			}
			if !found && msg.isNew {
				m.yaps = append([]Yap{*y}, m.yaps...)
				if m.tab == tabFeed && m.mode == modeList {
					m.cursor = 0
				}
			}
			if m.detail != nil && m.detail.Number == y.Number {
				yy := *y
				m.detail = &yy
				m.viewport.SetContent(m.renderDetail())
			}
		}
		if m.detail != nil {
			return m, m.loadReplies(m.detail.Number)
		}
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case modeCompose:
			return m.updateCompose(msg)
		case modeDetail:
			return m.updateDetail(msg)
		default:
			return m.updateList(msg)
		}
	}
	return m, nil
}

func (m model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.items()
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(items)-1 {
			m.cursor++
		}
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = len(items) - 1
	case "1", "2", "3", "4", "5":
		m.tab = tab(msg.String()[0] - '1')
		m.cursor, m.scroll, m.status = 0, 0, ""
	case "tab", "right":
		m.tab = (m.tab + 1) % tabCount
		m.cursor, m.scroll, m.status = 0, 0, ""
	case "shift+tab", "left":
		m.tab = (m.tab + tabCount - 1) % tabCount
		m.cursor, m.scroll, m.status = 0, 0, ""
	case "R", "ctrl+r":
		m.loading = true
		m.status = ""
		return m, m.load()
	case "enter", "v":
		if y := m.selected(); y != nil {
			yy := *y
			m.detail, m.replies, m.mode = &yy, nil, modeDetail
			m.viewport.SetContent(m.renderDetail())
			m.viewport.GotoTop()
			return m, m.loadReplies(y.Number)
		}
	case "l":
		if y := m.selected(); y != nil {
			return m, m.like(y.Number)
		}
	case "r":
		if y := m.selected(); y != nil {
			return m.startCompose(y.Number), nil
		}
	case "p":
		return m.startCompose(0), nil
	case "o":
		if y := m.selected(); y != nil {
			openBrowser(y.URL)
		} else {
			openBrowser(issuesURL(m.repo))
		}
	}
	m.clampCursor()
	return m, nil
}

func (m model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "backspace", "h", "left":
		m.mode, m.detail, m.replies = modeList, nil, nil
		return m, nil
	case "l":
		return m, m.like(m.detail.Number)
	case "r":
		return m.startCompose(m.detail.Number), nil
	case "o":
		openBrowser(m.detail.URL)
		return m, nil
	case "R", "ctrl+r":
		return m, tea.Batch(m.load(), m.loadReplies(m.detail.Number))
	case "1", "2", "3", "4":
		k := int(msg.String()[0] - '0')
		if m.detail.Poll == nil {
			m.status, m.statusOK = fmt.Sprintf(t("notpoll"), m.detail.Number), false
			return m, nil
		}
		if k > len(m.detail.Poll.Options) {
			return m, nil
		}
		n := m.detail.Number
		c := m.client
		return m, func() tea.Msg {
			if err := c.Vote(n, k); err != nil {
				return actionMsg{err: err}
			}
			return fresh(c, n, fmt.Sprintf(t("voted"), pollEmoji[k-1], n), false)
		}
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m model) startCompose(replyTo int) model {
	m.mode, m.composeFor, m.status = modeCompose, replyTo, ""
	m.composer.Reset()
	m.composer.Focus()
	return m
}

func (m model) updateCompose(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.composer.Blur()
		if m.detail != nil {
			m.mode = modeDetail
		} else {
			m.mode = modeList
		}
		return m, nil
	case "enter":
		text := strings.TrimSpace(m.composer.Value())
		if text == "" {
			return m, nil
		}
		if n := len([]rune(text)); n > maxLen {
			m.status, m.statusOK = fmt.Sprintf(t("toolong"), n, maxLen), false
			return m, nil
		}
		if looksSecret(text) {
			m.status, m.statusOK = t("secret"), false
			return m, nil
		}
		m.composer.Blur()
		if m.detail != nil {
			m.mode = modeDetail
		} else {
			m.mode = modeList
		}
		c, to := m.client, m.composeFor
		return m, func() tea.Msg {
			if to == 0 {
				n, err := c.Post(text)
				if err != nil {
					return actionMsg{err: err}
				}
				return fresh(c, n, fmt.Sprintf(t("posted"), n), true)
			}
			if err := c.Reply(to, text); err != nil {
				return actionMsg{err: err}
			}
			return fresh(c, to, fmt.Sprintf(t("replied"), to), false)
		}
	}
	var cmd tea.Cmd
	m.composer, cmd = m.composer.Update(msg)
	return m, cmd
}

func (m model) like(n int) tea.Cmd {
	c := m.client
	return func() tea.Msg {
		if err := c.Like(n); err != nil {
			return actionMsg{err: err}
		}
		return fresh(c, n, fmt.Sprintf(t("liked"), n), false)
	}
}

// items are the yaps shown on the current tab.
func (m model) items() []Yap {
	switch m.tab {
	case tabPolls:
		var out []Yap
		for _, y := range m.yaps {
			if y.Poll != nil {
				out = append(out, y)
			}
		}
		return out
	case tabMentions:
		return m.mentions
	case tabAgents:
		var out []Yap
		for _, y := range m.yaps {
			if y.Agent != "" {
				out = append(out, y)
			}
		}
		return out
	case tabTop:
		return nil
	}
	return m.yaps
}

func (m model) selected() *Yap {
	items := m.items()
	if m.cursor >= 0 && m.cursor < len(items) {
		return &items[m.cursor]
	}
	return nil
}

func (m *model) clampCursor() {
	n := len(m.items())
	if m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// ---------------------------------------------------------------------------
// view
// ---------------------------------------------------------------------------

func (m model) View() string {
	if m.width == 0 {
		return t("loading")
	}
	var body string
	switch m.mode {
	case modeDetail:
		body = m.viewport.View()
	case modeCompose:
		body = m.viewCompose()
	default:
		body = m.viewList()
	}
	frame := sFrame.Width(m.width - 2).Height(m.height - 5).Render(body)
	return lipgloss.JoinVertical(lipgloss.Left, m.viewHeader(), frame, m.viewFooter())
}

func (m model) viewHeader() string {
	live := sDim.Render("○ " + t("loading"))
	if !m.loading {
		live = sOK.Render("● ") + sDim.Render(t("live"))
	}
	left := sTitle.Render(" gh yap ") + sDim.Render("🏛️ "+m.repo)
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(live) - 1
	if gap < 1 {
		gap = 1
	}
	top := left + strings.Repeat(" ", gap) + live
	names := []string{t("tab.feed"), t("tab.polls"), t("tab.mentions"), t("tab.top"), "🤖 " + t("tab.agents")}
	var tabs []string
	for i, n := range names {
		label := fmt.Sprintf("%d %s", i+1, n)
		if tab(i) == m.tab {
			tabs = append(tabs, sTabOn.Render(label))
		} else {
			tabs = append(tabs, sTabOff.Render(label))
		}
	}
	return top + "\n" + strings.Join(tabs, "")
}

func (m model) viewFooter() string {
	keys := t("keys.list")
	switch m.mode {
	case modeDetail:
		keys = t("keys.detail")
	case modeCompose:
		keys = t("keys.compose")
	}
	line := sDim.Render(" " + keys)
	if m.err != nil {
		line = sErr.Render(" " + fmt.Sprintf(t("error"), m.err))
	} else if m.status != "" {
		st := sOK
		if !m.statusOK {
			st = sErr
		}
		line = st.Render(" "+m.status) + "  " + line
	}
	return truncate(line, m.width)
}

func (m model) viewList() string {
	w := m.width - 6
	h := m.height - 7
	if m.tab == tabTop {
		return m.viewTop(w)
	}
	items := m.items()
	if m.loading && len(m.yaps) == 0 {
		return sDim.Render(t("loading"))
	}
	if len(items) == 0 {
		key := "empty"
		switch m.tab {
		case tabPolls:
			key = "empty.polls"
		case tabMentions:
			key = "empty.ments"
		case tabAgents:
			key = "empty.agents"
		}
		out := sDim.Render(t(key))
		if m.tab == tabMentions {
			out += "\n\n" + m.renderMentionReplies(w)
		}
		return out
	}

	// render every card, remember where the selected one starts, scroll to keep it visible
	var lines []string
	selStart, selEnd := 0, 0
	for i, y := range items {
		card := strings.Split(renderCard(y, w-2, i == m.cursor), "\n")
		if i == m.cursor {
			selStart, selEnd = len(lines), len(lines)+len(card)
		}
		lines = append(lines, card...)
		lines = append(lines, "")
	}
	if m.tab == tabMentions && len(m.mentionR) > 0 {
		lines = append(lines, strings.Split(m.renderMentionReplies(w), "\n")...)
	}
	scroll := m.scroll
	if selStart < scroll {
		scroll = selStart
	}
	if selEnd > scroll+h {
		scroll = selEnd - h
	}
	if scroll < 0 {
		scroll = 0
	}
	end := scroll + h
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[scroll:end], "\n")
}

func (m model) renderMentionReplies(w int) string {
	var out []string
	for _, r := range m.mentionR {
		head := sDim.Render("└─ ") + sUser.Render("@"+r.Author)
		if r.Agent != "" {
			head += " " + sAgent.Render("🤖 "+r.Agent)
		}
		head += " " + sDim.Render("· "+ago(r.CreatedAt)+" · "+fmt.Sprintf(t("inreply"), r.Issue))
		out = append(out, head, indent(pretty(wrap(r.Text, w-7)), 7), "")
	}
	return strings.Join(out, "\n")
}

func (m model) viewTop(w int) string {
	board := leaderboard(m.yaps)
	if len(board) == 0 {
		return sDim.Render(t("empty"))
	}
	var out []string
	out = append(out, sDim.Render(t("karma")), "")
	for i, k := range board {
		rank := fmt.Sprintf("%2d.", i+1)
		name := sUser.Render(fmt.Sprintf("%-22s", "@"+k.Login))
		if i < 3 && k.Karma >= 3 {
			rank = "👑 "
			name = sCursor.Render(fmt.Sprintf("%-22s", "@"+k.Login))
		}
		out = append(out, fmt.Sprintf("  %s %s %s %s", rank, name, sHeart.Render(fmt.Sprintf("♥ %-5d", k.Karma)), sDim.Render(fmt.Sprintf("· %d yaps", k.Yaps))))
	}
	return strings.Join(out, "\n")
}

func (m model) viewCompose() string {
	title := t("compose.post")
	if m.composeFor > 0 {
		title = fmt.Sprintf(t("compose.reply"), m.composeFor)
	}
	n := len([]rune(m.composer.Value()))
	count := sDim.Render(fmt.Sprintf(t("chars"), n, maxLen))
	if n > maxLen {
		count = sErr.Render(fmt.Sprintf(t("chars"), n, maxLen))
	}
	box := sCompose.Width(m.width - 8).Render(m.composer.View())
	var context string
	if m.detail != nil {
		context = renderCard(*m.detail, m.width-8, false) + "\n\n"
	}
	return context + sTitle.Render(title) + "  " + count + "\n" + box
}

func (m model) renderDetail() string {
	if m.detail == nil {
		return ""
	}
	w := m.viewport.Width - 2
	out := []string{renderCard(*m.detail, w, false), ""}
	if m.detail.Replies == 0 && len(m.replies) == 0 {
		out = append(out, sDim.Render(t("noreplies")))
	} else {
		out = append(out, sDim.Render(fmt.Sprintf("%d %s", len(m.replies), t("replies"))), "")
	}
	for _, r := range m.replies {
		head := sDim.Render("└─ ") + sUser.Render("@"+r.Author)
		if r.Agent != "" {
			head += " " + sAgent.Render("🤖 "+r.Agent)
		}
		head += " " + sDim.Render("· "+ago(r.CreatedAt))
		out = append(out, head, indent(pretty(wrap(r.Text, w-7)), 7), "")
	}
	return strings.Join(out, "\n")
}

// renderCard draws one yap the way the bash script does, plus a cursor.
func renderCard(y Yap, w int, selected bool) string {
	cur := "  "
	if selected {
		cur = sCursor.Render("▸ ")
	}
	head := cur + sDim.Render(fmt.Sprintf("#%d", y.Number)) + " " + sUser.Render("@"+y.Author)
	if y.Agent != "" {
		head += " " + sAgent.Render("🤖 "+y.Agent)
	}
	head += " " + sDim.Render("· "+ago(y.CreatedAt))
	lines := []string{head}
	text := y.Text
	if y.Poll != nil {
		text = "📊 " + text
	}
	lines = append(lines, indent(pretty(wrap(text, w-6)), 6))
	if y.Poll != nil {
		total, max, width := 0, 0, 0
		for i, o := range y.Poll.Options {
			total += y.Poll.Votes[i]
			if y.Poll.Votes[i] > max {
				max = y.Poll.Votes[i]
			}
			if n := lipgloss.Width(o); n > width {
				width = n
			}
		}
		for i, o := range y.Poll.Options {
			f := 0
			if max > 0 {
				f = int(float64(y.Poll.Votes[i])*10/float64(max) + 0.5)
			}
			bar := sBar.Render(strings.Repeat("█", f) + strings.Repeat("░", 10-f))
			lines = append(lines, fmt.Sprintf("      %s %s  %s %d", pollEmoji[i], pad(o, width), bar, y.Poll.Votes[i]))
		}
		votes := fmt.Sprintf(t("votes"), total)
		if total == 1 {
			votes = t("vote1")
		}
		lines = append(lines, "      "+sDim.Render(votes))
	} else {
		lines = append(lines, "      "+sHeart.Render(fmt.Sprintf("♥ %d", y.Likes))+sDim.Render(fmt.Sprintf("  💬 %d", y.Replies)))
	}
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------------
// text helpers
// ---------------------------------------------------------------------------

func pretty(s string) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "> ") {
			out = append(out, sDim.Render(line))
			continue
		}
		line = mentionRe.ReplaceAllStringFunc(line, func(m string) string {
			sub := mentionRe.FindStringSubmatch(m)
			return sub[1] + lipgloss.NewStyle().Foreground(cMention).Render(sub[2])
		})
		line = tagRe.ReplaceAllStringFunc(line, func(m string) string {
			sub := tagRe.FindStringSubmatch(m)
			return sub[1] + lipgloss.NewStyle().Foreground(cTag).Render(sub[2])
		})
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func wrap(s string, w int) string {
	if w < 10 {
		w = 10
	}
	return lipgloss.NewStyle().Width(w).Render(s)
}

func indent(s string, n int) string {
	p := strings.Repeat(" ", n)
	return p + strings.ReplaceAll(s, "\n", "\n"+p)
}

func pad(s string, w int) string {
	if n := lipgloss.Width(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

func truncate(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(w).Render(s)
}

func ago(ts time.Time) string {
	d := time.Since(ts)
	switch {
	case d < time.Minute:
		if lang == "es" {
			return "ahora"
		}
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func openBrowser(url string) {
	for _, cmd := range [][]string{{"open", url}, {"xdg-open", url}, {"cmd", "/c", "start", url}} {
		if _, err := exec.LookPath(cmd[0]); err == nil {
			_ = exec.Command(cmd[0], cmd[1:]...).Start()
			return
		}
	}
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	repo := os.Getenv("GH_YAP_REPO")
	if repo == "" {
		repo = "josemingorance/yap"
	}
	flag.StringVar(&repo, "repo", repo, "square repo (owner/name)")
	showVersion := flag.Bool("version", false, "print version")
	flag.Parse()
	if *showVersion {
		fmt.Println("gh-yap-tui", version)
		return
	}
	detectLang()

	client, err := newClient(repo)
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗", err, "\n  (is gh logged in? try: gh auth login)")
		os.Exit(1)
	}
	p := tea.NewProgram(newModel(client, repo), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "✗", err)
		os.Exit(1)
	}
}
