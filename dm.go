package main

// Encrypted direct messages.
//
// Messages are encrypted with age (https://age-encryption.org) to the SSH
// public keys that each user already publishes on GitHub
// (github.com/<user>.keys), and stored as comments in a public repo.
//
//   - Only the recipient (and the sender) can read the content.
//   - Who talks to whom, when and how much is PUBLIC metadata.
//   - The content is as safe as the users' SSH private keys. No forward
//     secrecy: a leaked key reveals that key's history.
//   - AI agents must never use this: commands refuse to run without an
//     interactive terminal or when GH_YAP_AGENT is set.

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"filippo.io/age"
	"filippo.io/age/agessh"
	"filippo.io/age/armor"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

const (
	dmMsgMarker = "<!-- gh-yap:dm-msg -->"
	dmMaxLen    = 1000
	dmInfoURL   = "https://github.com/josemingorance/gh-yap#-private-messages"
)

var dmPairRe = regexp.MustCompile(`<!-- gh-yap:dm=([a-z0-9-]+),([a-z0-9-]+) -->`)

func dmRepo() string {
	if r := os.Getenv("GH_YAP_DM_REPO"); r != "" {
		return r
	}
	return "josemingorance/yap-dm"
}

// pair returns the two logins, lowercased and sorted, so A→B and B→A share a thread.
func pair(a, b string) (string, string) {
	a, b = strings.ToLower(a), strings.ToLower(b)
	if a > b {
		a, b = b, a
	}
	return a, b
}

func pairMarker(a, b string) string {
	a, b = pair(a, b)
	return fmt.Sprintf("<!-- gh-yap:dm=%s,%s -->", a, b)
}

// ---------------------------------------------------------------------------
// keys
// ---------------------------------------------------------------------------

// recipientKeys returns the age recipients for a user's GitHub SSH keys.
// Only ed25519 and RSA keys work with age; FIDO (sk-) and ECDSA keys are skipped.
func (c *Client) recipientKeys(login string) ([]age.Recipient, []string, error) {
	lines, err := c.publicKeys(login)
	if err != nil {
		return nil, nil, err
	}
	var rs []age.Recipient
	var used []string
	for _, line := range lines {
		r, err := agessh.ParseRecipient(line)
		if err != nil {
			continue
		}
		pk, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
		if err != nil {
			continue
		}
		rs = append(rs, r)
		used = append(used, pk.Type()+" "+ssh.FingerprintSHA256(pk))
	}
	return rs, used, nil
}

func (c *Client) publicKeys(login string) ([]string, error) {
	if f := os.Getenv("GH_YAP_DM_TEST_KEYS"); f != "" { // testing hook: "login key..." per line
		// A swapped key file would silently redirect messages to someone else's key,
		// so it only works against a non-default repo and always says so.
		if os.Getenv("GH_YAP_DM_REPO") == "" || dmRepo() == "josemingorance/yap-dm" {
			return nil, errors.New("GH_YAP_DM_TEST_KEYS only works together with a test GH_YAP_DM_REPO")
		}
		fmt.Fprintln(os.Stderr, sErr.Render("⚠️  using TEST keys from "+f+", not GitHub"))
		return testKeys(f, login)
	}
	var keys []struct {
		Key string `json:"key"`
	}
	if err := c.rest.Get("users/"+login+"/keys", &keys); err != nil {
		return nil, err
	}
	var out []string
	for _, k := range keys {
		out = append(out, k.Key)
	}
	return out, nil
}

func testKeys(path, login string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.SplitN(strings.TrimSpace(sc.Text()), " ", 2)
		if len(fields) == 2 && strings.EqualFold(fields[0], login) {
			out = append(out, fields[1])
		}
	}
	return out, sc.Err()
}

// loadIdentity reads the local SSH private key used to decrypt.
// GH_YAP_SSH_KEY picks the file; otherwise ~/.ssh/id_ed25519, then ~/.ssh/id_rsa.
// A passphrase-protected key is unlocked through the passphrase callback.
func loadIdentity(passphrase func() ([]byte, error)) (age.Identity, string, error) {
	var candidates []string
	if p := os.Getenv("GH_YAP_SSH_KEY"); p != "" {
		candidates = []string{p}
	} else if home, err := os.UserHomeDir(); err == nil {
		candidates = []string{filepath.Join(home, ".ssh", "id_ed25519"), filepath.Join(home, ".ssh", "id_rsa")}
	}
	for _, path := range candidates {
		pemBytes, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		id, err := agessh.ParseIdentity(pemBytes)
		if err == nil {
			return id, path, nil
		}
		var missing *ssh.PassphraseMissingError
		if !errors.As(err, &missing) {
			return nil, path, err
		}
		pub := missing.PublicKey
		if pub == nil {
			pubBytes, err := os.ReadFile(path + ".pub")
			if err != nil {
				return nil, path, fmt.Errorf("%s is passphrase-protected and %s.pub is missing", path, path)
			}
			if pub, _, _, _, err = ssh.ParseAuthorizedKey(pubBytes); err != nil {
				return nil, path, err
			}
		}
		if passphrase == nil {
			return nil, path, errors.New(t("dm.needpass"))
		}
		pw, err := passphrase()
		if err != nil {
			return nil, path, err
		}
		if _, err := ssh.ParseRawPrivateKeyWithPassphrase(pemBytes, pw); err != nil {
			return nil, path, errors.New(t("dm.badpass"))
		}
		id2, err := agessh.NewEncryptedSSHIdentity(pub, pemBytes, func() ([]byte, error) { return pw, nil })
		return id2, path, err
	}
	return nil, "", errors.New(t("dm.nokey"))
}

// localKeyFingerprint reads the public half of the local key (never the private one).
func localKeyFingerprint() (string, string) {
	var candidates []string
	if p := os.Getenv("GH_YAP_SSH_KEY"); p != "" {
		candidates = []string{p}
	} else if home, err := os.UserHomeDir(); err == nil {
		candidates = []string{filepath.Join(home, ".ssh", "id_ed25519"), filepath.Join(home, ".ssh", "id_rsa")}
	}
	for _, path := range candidates {
		b, err := os.ReadFile(path + ".pub")
		if err != nil {
			continue
		}
		if pk, _, _, _, err := ssh.ParseAuthorizedKey(b); err == nil {
			return ssh.FingerprintSHA256(pk), path
		}
	}
	return "", ""
}

func ttyPassphrase() ([]byte, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	defer tty.Close()
	fmt.Fprint(tty, t("dm.passphrase"))
	pw, err := term.ReadPassword(int(tty.Fd()))
	fmt.Fprintln(tty)
	return pw, err
}

// ---------------------------------------------------------------------------
// crypto
// ---------------------------------------------------------------------------

func encrypt(text string, recipients ...age.Recipient) (string, error) {
	var buf bytes.Buffer
	aw := armor.NewWriter(&buf)
	w, err := age.Encrypt(aw, recipients...)
	if err != nil {
		return "", err
	}
	if _, err := io.WriteString(w, text); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	if err := aw.Close(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

var armorRe = regexp.MustCompile(`(?s)-----BEGIN AGE ENCRYPTED FILE-----.*?-----END AGE ENCRYPTED FILE-----`)

func decrypt(body string, id age.Identity) (string, error) {
	block := armorRe.FindString(body)
	if block == "" {
		return "", errors.New("no encrypted block")
	}
	r, err := age.Decrypt(armor.NewReader(strings.NewReader(block)), id)
	if err != nil {
		return "", err
	}
	out, err := io.ReadAll(io.LimitReader(r, 64*1024))
	return string(out), err
}

// ---------------------------------------------------------------------------
// storage: one issue per pair in the DM repo, one comment per message
// ---------------------------------------------------------------------------

// Conversation is a DM thread between two people.
type Conversation struct {
	Number int
	With   string
	A, B   string
}

// DM is one message; Text is empty until decrypted.
type DM struct {
	From      string
	CreatedAt time.Time
	Body      string
	Text      string
	Err       error
}

// conversations lists the threads that involve me. An issue only counts when
// one of the two participants opened it: anyone can open issues in a public
// repo, so a stranger's issue is ignored.
func (c *Client) conversations() ([]Conversation, error) {
	// Ask GitHub only for threads I opened or am mentioned in (the thread body
	// mentions both people), so junk issues from strangers can't push mine away.
	var issues []ghIssue
	seen := map[int]bool{}
	for _, filter := range []string{"creator", "mentioned"} {
		for page := 1; page <= 10; page++ {
			var batch []ghIssue
			path := fmt.Sprintf("repos/%s/issues?state=all&%s=%s&sort=created&direction=asc&per_page=100&page=%d", dmRepo(), filter, c.Me, page)
			if err := c.rest.Get(path, &batch); err != nil {
				return nil, err
			}
			for _, i := range batch {
				if !seen[i.Number] {
					seen[i.Number] = true
					issues = append(issues, i)
				}
			}
			if len(batch) < 100 {
				break
			}
		}
	}
	sort.Slice(issues, func(a, b int) bool { return issues[a].Number < issues[b].Number })
	return filterConversations(issues, c.Me), nil
}

func filterConversations(issues []ghIssue, me string) []Conversation {
	me = strings.ToLower(me)
	seen := map[string]bool{}
	var out []Conversation
	for _, i := range issues {
		m := dmPairRe.FindStringSubmatch(i.Body)
		if m == nil || i.PullRequest != nil {
			continue
		}
		a, b := m[1], m[2]
		author := strings.ToLower(i.User.Login)
		if (a != me && b != me) || (author != a && author != b) || seen[a+","+b] {
			continue
		}
		seen[a+","+b] = true
		with := a
		if a == me {
			with = b
		}
		out = append(out, Conversation{Number: i.Number, With: with, A: a, B: b})
	}
	return out
}

func (c *Client) conversationWith(login string) (*Conversation, error) {
	cs, err := c.conversations()
	if err != nil {
		return nil, err
	}
	for _, cv := range cs {
		if strings.EqualFold(cv.With, login) {
			cv := cv
			return &cv, nil
		}
	}
	return nil, nil
}

func (c *Client) openConversation(login string) (*Conversation, error) {
	if cv, err := c.conversationWith(login); err != nil || cv != nil {
		return cv, err
	}
	a, b := pair(c.Me, login)
	body := fmt.Sprintf("🔐 Private messages between @%s and @%s, sent with [gh-yap](%s).\n\n"+
		"Every comment here is encrypted to the participants' SSH keys: only they can read it. "+
		"Who talks to whom, and when, is public.\n\n%s", a, b, homeURL, pairMarker(a, b))
	var out struct {
		Number int `json:"number"`
	}
	title := fmt.Sprintf("🔐 @%s ↔ @%s", a, b)
	if err := c.post(fmt.Sprintf("repos/%s/issues", dmRepo()), map[string]string{"title": title, "body": body}, &out); err != nil {
		return nil, err
	}
	with := a
	if a == strings.ToLower(c.Me) {
		with = b
	}
	return &Conversation{Number: out.Number, With: with, A: a, B: b}, nil
}

// messages returns a thread's messages, keeping only comments by the two participants.
func (c *Client) messages(cv *Conversation) ([]DM, error) {
	var comments []ghIssue
	if err := c.rest.Get(fmt.Sprintf("repos/%s/issues/%d/comments?per_page=100", dmRepo(), cv.Number), &comments); err != nil {
		return nil, err
	}
	return filterMessages(comments, cv), nil
}

func filterMessages(comments []ghIssue, cv *Conversation) []DM {
	var out []DM
	for _, cm := range comments {
		from := strings.ToLower(cm.User.Login)
		if (from != cv.A && from != cv.B) || !strings.Contains(cm.Body, dmMsgMarker) {
			continue
		}
		out = append(out, DM{From: cm.User.Login, CreatedAt: cm.CreatedAt, Body: cm.Body})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

// sendDM encrypts text to the recipient's keys and to mine, and posts it.
func (c *Client) sendDM(to, text string, dryRun bool) (string, error) {
	theirs, theirUsed, err := c.recipientKeys(to)
	if err != nil {
		return "", err
	}
	if len(theirs) == 0 {
		return "", fmt.Errorf(t("dm.nokeys"), to)
	}
	mine, myUsed, err := c.recipientKeys(c.Me)
	if err != nil {
		return "", err
	}
	if len(mine) == 0 {
		return "", fmt.Errorf(t("dm.nokeys"), c.Me)
	}
	if fp, path := localKeyFingerprint(); fp != "" && !strings.Contains(strings.Join(myUsed, " "), fp) {
		fmt.Fprintln(os.Stderr, sErr.Render(fmt.Sprintf(t("dm.keymismatch"), shortPath(path))))
	}
	ct, err := encrypt(text, append(theirs, mine...)...)
	if err != nil {
		return "", err
	}
	a, b := pair(c.Me, to)
	reader := "@" + a
	if a != b {
		reader += " & @" + b
	}
	body := fmt.Sprintf("🔐 encrypted message · only %s can read it · [what is this?](%s)\n\n```\n%s```\n%s", reader, dmInfoURL, ct, dmMsgMarker)
	if dryRun {
		return fmt.Sprintf(t("dm.dryrun"), len(theirUsed), to, strings.Join(theirUsed, "\n    ")), nil
	}
	cv, err := c.openConversation(to)
	if err != nil {
		return "", err
	}
	if err := c.post(fmt.Sprintf("repos/%s/issues/%d/comments", dmRepo(), cv.Number), map[string]string{"body": body}, nil); err != nil {
		return "", err
	}
	return fmt.Sprintf(t("dm.sent"), to) + "\n    " + sDim.Render(strings.Join(theirUsed, "\n    ")), nil
}

// ---------------------------------------------------------------------------
// CLI: gh yap-tui dm <list|read|send|keys>
// ---------------------------------------------------------------------------

func ackFile() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "gh-yap", "dm-ok")
}

// firstRunNotice shows what DMs do and don't protect, once, and asks for a yes.
func firstRunNotice() error {
	f := ackFile()
	if f == "" {
		return nil
	}
	if _, err := os.Stat(f); err == nil {
		return nil
	}
	fmt.Println(t("dm.notice"))
	fmt.Print(t("dm.confirm"))
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	if ans := strings.ToLower(strings.TrimSpace(line)); ans != "yes" && ans != "si" && ans != "sí" && ans != "y" && ans != "s" {
		return errors.New(t("dm.declined"))
	}
	_ = os.MkdirAll(filepath.Dir(f), 0o700)
	return os.WriteFile(f, []byte(time.Now().Format(time.RFC3339)+"\n"), 0o600)
}

func dmGuard(needTTY bool) error {
	if os.Getenv("GH_YAP_AGENT") != "" {
		return errors.New(t("dm.agent"))
	}
	if needTTY && !term.IsTerminal(int(os.Stdin.Fd())) {
		return errors.New(t("dm.tty"))
	}
	return nil
}

func runDM(c *Client, args []string) error {
	if len(args) == 0 {
		fmt.Println(t("dm.usage"))
		return nil
	}
	sub, rest := args[0], args[1:]
	var who string
	var words []string
	dryRun, literal := false, false
	for _, a := range rest {
		switch {
		case literal:
			words = append(words, a)
		case a == "--":
			literal = true
		case a == "--dry-run":
			dryRun = true
		case a == "-h" || a == "--help":
			fmt.Println(t("dm.usage"))
			return nil
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf(t("dm.badflag"), a)
		case who == "" && sub != "list":
			who = strings.TrimPrefix(a, "@")
		default:
			words = append(words, a)
		}
	}
	if who != "" && !regexp.MustCompile(`^[A-Za-z0-9-]{1,39}$`).MatchString(who) {
		return fmt.Errorf(t("dm.baduser"), who)
	}

	switch sub {
	case "list", "ls": // metadata only (it's public anyway), so no terminal needed
		if err := dmGuard(false); err != nil {
			return err
		}
		cs, err := c.conversations()
		if err != nil {
			return err
		}
		if len(cs) == 0 {
			fmt.Println(sDim.Render(t("dm.none")))
			return nil
		}
		for _, cv := range cs {
			msgs, _ := c.messages(&cv)
			last := ""
			if len(msgs) > 0 {
				last = " · " + ago(msgs[len(msgs)-1].CreatedAt)
			}
			fmt.Printf("  🔐 %s %s\n", sUser.Render("@"+cv.With), sDim.Render(fmt.Sprintf(t("dm.count"), len(msgs))+last))
		}
		return nil

	case "keys":
		if who == "" {
			return errors.New(t("dm.usage"))
		}
		_, used, err := c.recipientKeys(who)
		if err != nil {
			return err
		}
		if len(used) == 0 {
			return fmt.Errorf(t("dm.nokeys"), who)
		}
		fmt.Printf(t("dm.keysfor")+"\n", who)
		for _, u := range used {
			fmt.Println("    " + u)
		}
		return nil

	case "send":
		if err := dmGuard(true); err != nil {
			return err
		}
		text := strings.TrimSpace(strings.Join(words, " "))
		if who == "" || text == "" {
			return errors.New(t("dm.usage"))
		}
		if n := len([]rune(text)); n > dmMaxLen {
			return fmt.Errorf(t("toolong"), n, dmMaxLen)
		}
		if err := firstRunNotice(); err != nil {
			return err
		}
		msg, err := c.sendDM(who, text, dryRun)
		if err != nil {
			return err
		}
		fmt.Println(sOK.Render("✓ ") + msg)
		return nil

	case "read":
		if err := dmGuard(true); err != nil {
			return err
		}
		if who == "" {
			return errors.New(t("dm.usage"))
		}
		if err := firstRunNotice(); err != nil {
			return err
		}
		cv, err := c.conversationWith(who)
		if err != nil {
			return err
		}
		if cv == nil {
			fmt.Println(sDim.Render(fmt.Sprintf(t("dm.empty"), who)))
			return nil
		}
		msgs, err := c.messages(cv)
		if err != nil {
			return err
		}
		id, path, err := loadIdentity(ttyPassphrase)
		if err != nil {
			return err
		}
		fmt.Println(sDim.Render(fmt.Sprintf(t("dm.header"), who, shortPath(path))) + "\n")
		for _, m := range msgs {
			text, err := decrypt(m.Body, id)
			if err != nil {
				text = sErr.Render(t("dm.cantread"))
			}
			fmt.Printf("%s %s\n%s\n\n", sUser.Render("@"+m.From), sDim.Render("· "+ago(m.CreatedAt)), trimLines(indent(wrap(text, 70), 4)))
		}
		return nil
	}
	return errors.New(t("dm.usage"))
}

func trimLines(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	return strings.Join(lines, "\n")
}
