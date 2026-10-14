package main

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func sample() []Yap {
	now := time.Now()
	return []Yap{
		{Number: 18, Author: "github-actions[bot]", Agent: "square-bot", Text: "📰 digest", CreatedAt: now},
		{Number: 15, Author: "jose", Text: "¿tabs?", CreatedAt: now, Poll: &Poll{Options: []string{"a", "b"}, Votes: []int{1, 0}}},
		{Number: 14, Author: "jose", Agent: "claude-code", Text: "hello", Likes: 6, CreatedAt: now.Add(-48 * time.Hour)},
		{Number: 6, Author: "ana", Text: "🔁 @jose #3: quoted", Likes: 1, CreatedAt: now},
	}
}

func TestEveryTabRenders(t *testing.T) {
	for _, l := range []string{"en", "es"} {
		lang = l
		m := newModel(&Client{Me: "jose"}, "o/r")
		m.loading = false
		m.yaps = sample()
		mm, _ := m.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
		m = mm.(model)
		for tb := tab(0); tb < tabCount; tb++ {
			m.tab = tb
			done := make(chan string, 1)
			go func() { done <- m.View() }()
			select {
			case out := <-done:
				if strings.TrimSpace(out) == "" {
					t.Fatalf("%s tab %d rendered empty", l, tb)
				}
				if tb == tabTop && !strings.Contains(out, "@jose") {
					t.Fatalf("top tab missing user:\n%s", out)
				}
				if tb == tabTop && strings.Contains(out, "github-actions") {
					t.Fatalf("bot shown in top")
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("%s tab %d hangs in View()", l, tb)
			}
		}
	}
}

func TestBadges(t *testing.T) {
	b := strings.Join(badges("jose", sample(), true), "")
	for _, want := range []string{"🐣", "📊", "❤️", "🔁", "🤖", "👑"} {
		if !strings.Contains(b, want) {
			t.Errorf("missing %s in %s", want, b)
		}
	}
}
