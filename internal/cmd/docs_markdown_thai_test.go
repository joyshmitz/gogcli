package cmd

import (
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/docsmarkdown"
)

// withTimeout runs fn in a goroutine and fails the test if it does not return
// within d. Used to catch infinite loops without blocking the whole test run.
func withTimeout(t *testing.T, d time.Duration, name string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("%s: timed out after %s (suspected infinite loop)", name, d)
	}
}

// TestMarkdownToDocsRequests_ThaiAppend exercises the full path used by
// `gog docs write --markdown --append <thai-md>`: parse markdown, then convert
// to Docs API requests at a non-zero base index. Each input ends in a Thai
// rune, which is the trigger condition for the original hang.
func TestMarkdownToDocsRequests_ThaiAppend(t *testing.T) {
	const sample = `## ส่วนคำถาม

คำถามที่พบบ่อยของลูกค้า

- ราคาเท่าไหร่
- ส่งของเมื่อไหร่

> ติดต่อสอบถามเพิ่มเติม
`
	withTimeout(t, 5*time.Second, "MarkdownToDocsRequests", func() {
		elements := docsmarkdown.ParseMarkdown(sample)
		if len(elements) == 0 {
			t.Fatal("ParseMarkdown returned no elements for Thai sample")
		}
		// baseIndex = 100 mimics appending at the tail of an existing doc.
		reqs, plain, _ := MarkdownToDocsRequests(elements, 100, "")
		if plain == "" {
			t.Fatal("MarkdownToDocsRequests returned empty plain text for Thai sample")
		}
		if len(reqs) == 0 {
			t.Fatal("MarkdownToDocsRequests returned no requests for Thai sample")
		}
	})
}
