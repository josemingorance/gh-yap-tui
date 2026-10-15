package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
	"filippo.io/age/agessh"
	"golang.org/x/crypto/ssh"
)

// throwaway key pair: returns the authorized_keys line and the private key PEM
func newKey(t *testing.T, passphrase string) (string, []byte) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sp, _ := ssh.NewPublicKey(pub)
	var block *pem.Block
	if passphrase == "" {
		block, err = ssh.MarshalPrivateKey(priv, "test")
	} else {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(priv, "test", []byte(passphrase))
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sp))), pem.EncodeToMemory(block)
}

func recipient(t *testing.T, line string) age.Recipient {
	r, err := agessh.ParseRecipient(line)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func identity(t *testing.T, pemBytes []byte) age.Identity {
	id, err := agessh.ParseIdentity(pemBytes)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestOnlyParticipantsCanRead(t *testing.T) {
	aPub, aPriv := newKey(t, "")
	bPub, bPriv := newKey(t, "")
	_, cPriv := newKey(t, "")

	ct, err := encrypt("hola, esto es privado 🤫", recipient(t, aPub), recipient(t, bPub))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ct, "privado") {
		t.Fatal("plaintext leaked into the ciphertext")
	}
	body := "🔐 encrypted message\n\n```\n" + ct + "```\n" + dmMsgMarker
	for name, priv := range map[string][]byte{"sender": aPriv, "recipient": bPriv} {
		got, err := decrypt(body, identity(t, priv))
		if err != nil || got != "hola, esto es privado 🤫" {
			t.Fatalf("%s couldn't read it: %q %v", name, got, err)
		}
	}
	if got, err := decrypt(body, identity(t, cPriv)); err == nil {
		t.Fatalf("a third party decrypted it: %q", got)
	}
}

func TestPassphraseProtectedKey(t *testing.T) {
	pub, priv := newKey(t, "correct horse")
	dir := t.TempDir()
	path := filepath.Join(dir, "id_ed25519")
	os.WriteFile(path, priv, 0o600)
	os.WriteFile(path+".pub", []byte(pub+"\n"), 0o644)
	t.Setenv("GH_YAP_SSH_KEY", path)

	ct, _ := encrypt("secret", recipient(t, pub))

	id, _, err := loadIdentity(func() ([]byte, error) { return []byte("correct horse"), nil })
	if err != nil {
		t.Fatal(err)
	}
	if got, err := decrypt(ct, id); err != nil || got != "secret" {
		t.Fatalf("right passphrase failed: %q %v", got, err)
	}

	id, _, err = loadIdentity(func() ([]byte, error) { return []byte("wrong"), nil })
	if err == nil {
		if _, err := decrypt(ct, id); err == nil {
			t.Fatal("wrong passphrase decrypted the message")
		}
	}
}

func TestStrangersAreIgnored(t *testing.T) {
	issue := func(n int, author, a, b string) ghIssue {
		return ghIssue{Number: n, User: ghUser{Login: author}, Body: "x\n\n" + pairMarker(a, b)}
	}
	issues := []ghIssue{
		issue(1, "mallory", "ana", "jose"), // a stranger opening our thread: ignored
		issue(2, "Jose", "ana", "jose"),    // the real one (case-insensitive)
		issue(3, "ana", "ana", "jose"),     // duplicate: the oldest real one wins
		issue(4, "bob", "bob", "carol"),    // not mine
	}
	cs := filterConversations(issues, "jose")
	if len(cs) != 1 || cs[0].Number != 2 || cs[0].With != "ana" {
		t.Fatalf("got %+v", cs)
	}

	now := time.Now()
	comments := []ghIssue{
		{User: ghUser{Login: "mallory"}, Body: "fake " + dmMsgMarker, CreatedAt: now},
		{User: ghUser{Login: "ana"}, Body: "real " + dmMsgMarker, CreatedAt: now.Add(time.Second)},
		{User: ghUser{Login: "jose"}, Body: "no marker", CreatedAt: now},
	}
	msgs := filterMessages(comments, &cs[0])
	if len(msgs) != 1 || msgs[0].From != "ana" {
		t.Fatalf("got %+v", msgs)
	}
}

func TestPairIsSymmetric(t *testing.T) {
	if pairMarker("Jose", "ana") != pairMarker("ANA", "jose") {
		t.Fatal("A→B and B→A must share a thread")
	}
}

func TestAgentsAreRefused(t *testing.T) {
	t.Setenv("GH_YAP_AGENT", "claude-code")
	if err := dmGuard(false); err == nil {
		t.Fatal("agents must not use DMs")
	}
	t.Setenv("GH_YAP_AGENT", "")
	if err := dmGuard(true); err == nil {
		t.Fatal("without a terminal, DMs must refuse") // go test has no TTY on stdin
	}
}
