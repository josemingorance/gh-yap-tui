package main

// The 🔐 tab: encrypted private messages inside the TUI.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"filippo.io/age"
	"filippo.io/age/agessh"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/crypto/ssh"
)

type dmState struct {
	convs  []Conversation
	counts map[int]int
	last   map[int]time.Time
	loaded bool

	with    string
	thread  []DM
	id      age.Identity
	keyPath string
	pass    textinput.Model
	pending string // who we were opening when we stopped to ask for the passphrase
}

type dmConvsMsg struct {
	convs  []Conversation
	counts map[int]int
	last   map[int]time.Time
	err    error
}
type dmThreadMsg struct {
	with   string
	thread []DM
	err    error
}

func newPassInput() textinput.Model {
	p := textinput.New()
	p.EchoMode = textinput.EchoPassword
	p.EchoCharacter = '•'
	p.Prompt = "🔑 "
	return p
}

func (m model) loadDMConvs() tea.Cmd {
	c := m.dmClient
	return func() tea.Msg {
		cs, err := c.conversations()
		if err != nil {
			return dmConvsMsg{err: err}
		}
		counts, last := map[int]int{}, map[int]time.Time{}
		for _, cv := range cs {
			cv := cv
			if msgs, err := c.messages(&cv); err == nil {
				counts[cv.Number] = len(msgs)
				if len(msgs) > 0 {
					last[cv.Number] = msgs[len(msgs)-1].CreatedAt
				}
			}
		}
		return dmConvsMsg{convs: cs, counts: counts, last: last}
	}
}

func (m model) loadDMThread(with string) tea.Cmd {
	c, id := m.dmClient, m.dm.id
	return func() tea.Msg {
		cv, err := c.conversationWith(with)
		if err != nil || cv == nil {
			return dmThreadMsg{with: with, err: err}
		}
		msgs, err := c.messages(cv)
		if err != nil {
			return dmThreadMsg{with: with, err: err}
		}
		for i := range msgs {
			msgs[i].Text, msgs[i].Err = decrypt(msgs[i].Body, id)
		}
		return dmThreadMsg{with: with, thread: msgs}
	}
}

func dmAcked() bool {
	f := ackFile()
	if f == "" {
		return true
	}
	_, err := os.Stat(f)
	return err == nil
}

// keyNeedsPassphrase tells whether the local key is protected, so the TUI can ask first.
func keyNeedsPassphrase() (bool, error) {
	var candidates []string
	if p := os.Getenv("GH_YAP_SSH_KEY"); p != "" {
		candidates = []string{p}
	} else if home, err := os.UserHomeDir(); err == nil {
		candidates = []string{filepath.Join(home, ".ssh", "id_ed25519"), filepath.Join(home, ".ssh", "id_rsa")}
	}
	for _, path := range candidates {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		_, err = agessh.ParseIdentity(b)
		var missing *ssh.PassphraseMissingError
		return errors.As(err, &missing), nil
	}
	return false, errors.New(t("dm.nokey"))
}

// openDM starts the flow to read the thread with someone: notice → passphrase → thread.
func (m model) openDM(with string) (tea.Model, tea.Cmd) {
	if os.Getenv("GH_YAP_AGENT") != "" {
		m.status, m.statusOK = t("dm.agent"), false
		return m, nil
	}
	m.dm.pending = with
	if !dmAcked() {
		m.mode = modeDMNotice
		return m, nil
	}
	if m.dm.id == nil {
		needs, err := keyNeedsPassphrase()
		if err != nil {
			m.status, m.statusOK = err.Error(), false
			return m, nil
		}
		if needs {
			m.dm.pass = newPassInput()
			m.dm.pass.Focus()
			m.mode = modeDMPass
			return m, textinput.Blink
		}
		id, path, err := loadIdentity(nil)
		if err != nil {
			m.status, m.statusOK = err.Error(), false
			return m, nil
		}
		m.dm.id, m.dm.keyPath = id, path
	}
	m.mode, m.dm.with, m.dm.thread = modeDMThread, with, nil
	return m, m.loadDMThread(with)
}

func (m model) updateDMNotice(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "y", "s", "Y", "S":
		f := ackFile()
		_ = os.MkdirAll(filepath.Dir(f), 0o700)
		_ = os.WriteFile(f, []byte(time.Now().Format(time.RFC3339)+"\n"), 0o600)
		m.mode = modeList
		return m.openDM(m.dm.pending)
	default:
		m.mode = modeList
		m.status, m.statusOK = t("dm.declined"), false
	}
	return m, nil
}

func (m model) updateDMPass(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode = modeList
		return m, nil
	case "enter":
		pw := []byte(m.dm.pass.Value())
		m.dm.pass.Reset()
		id, path, err := loadIdentity(func() ([]byte, error) { return pw, nil })
		if err != nil {
			m.mode, m.status, m.statusOK = modeList, err.Error(), false
			return m, nil
		}
		m.dm.id, m.dm.keyPath = id, path
		m.mode = modeList
		return m.openDM(m.dm.pending)
	}
	var cmd tea.Cmd
	m.dm.pass, cmd = m.dm.pass.Update(msg)
	return m, cmd
}

func (m model) updateDMThread(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q":
		return m, tea.Quit
	case "esc", "backspace", "left", "h":
		m.mode = modeList
		return m, m.loadDMConvs()
	case "r", "m":
		m = m.startCompose(0)
		m.composeDM = m.dm.with
		m.composer.CharLimit = dmMaxLen
		return m, nil
	case "R", "ctrl+r":
		return m, m.loadDMThread(m.dm.with)
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m model) sendDMCmd(to, text string) tea.Cmd {
	c := m.dmClient
	return func() tea.Msg {
		msg, err := c.sendDM(to, text, false)
		if err != nil {
			return actionMsg{err: err}
		}
		return dmSentMsg{text: msg, to: to}
	}
}

type dmSentMsg struct{ text, to string }

// ---------------------------------------------------------------------------
// views
// ---------------------------------------------------------------------------

func (m model) viewDMList() string {
	if os.Getenv("GH_YAP_AGENT") != "" {
		return sErr.Render(t("dm.agent"))
	}
	if !m.dm.loaded {
		return sDim.Render(t("loading"))
	}
	var out []string
	out = append(out, sDim.Render(t("dm.tabhint")), "")
	if len(m.dm.convs) == 0 {
		out = append(out, sDim.Render(t("dm.none")))
		return strings.Join(out, "\n")
	}
	for i, cv := range m.dm.convs {
		cur := "  "
		if i == m.cursor {
			cur = sCursor.Render("▸ ")
		}
		meta := fmt.Sprintf(t("dm.count"), m.dm.counts[cv.Number])
		if ts, ok := m.dm.last[cv.Number]; ok {
			meta += " · " + ago(ts)
		}
		out = append(out, cur+"🔐 "+sUser.Render("@"+cv.With)+"  "+sDim.Render(meta))
	}
	return strings.Join(out, "\n")
}

func (m model) renderDMThread() string {
	w := m.viewport.Width - 2
	out := []string{sDim.Render(fmt.Sprintf(t("dm.header"), m.dm.with, shortPath(m.dm.keyPath))), ""}
	if len(m.dm.thread) == 0 {
		out = append(out, sDim.Render(fmt.Sprintf(t("dm.empty"), m.dm.with)+" · r"))
	}
	for _, d := range m.dm.thread {
		text := d.Text
		if d.Err != nil {
			text = sErr.Render(t("dm.cantread"))
		}
		out = append(out, sUser.Render("@"+d.From)+" "+sDim.Render("· "+ago(d.CreatedAt)),
			trimLines(indent(wrap(text, w-6), 4)), "")
	}
	return strings.Join(out, "\n")
}

func (m model) viewDMNotice() string {
	return t("dm.notice") + "\n" + sTitle.Render(t("dm.tuiconfirm"))
}

func (m model) viewDMPass() string {
	return sDim.Render(t("dm.passphrase")) + "\n\n" + m.dm.pass.View()
}

func shortPath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home+string(os.PathSeparator)) {
		return "~" + p[len(home):]
	}
	return p
}
