package main

import (
	"strings"
	"testing"
)

func TestParseSiteStripsEscapes(t *testing.T) {
	idx := []byte(`<html><body><div class="floating-box center"><h2 id="welcome">Hi\x1b[31m</h2><p>text <a href="javascript:alert(1)">bad</a> <a href="https://ok.example/\x1b]52;x">ok</a>\x07</p></div>
<a class="social-item" href="https://github.com/x">GitHub</a></body></html>`)
	secs, err := parseSite([]byte(strings.ReplaceAll(string(idx), `\x1b`, "\x1b")), []byte(`<html></html>`))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range secs {
		for _, l := range s.Links {
			if strings.ContainsAny(l.URL+l.Text, "\x1b\x07") {
				t.Fatalf("control character left in link %q", l.URL)
			}
			if strings.HasPrefix(l.URL, "javascript:") {
				t.Fatalf("unsafe scheme kept: %q", l.URL)
			}
		}
		for _, para := range s.Paragraphs {
			for _, sp := range para {
				if strings.ContainsAny(sp.Text, "\x1b\x07") {
					t.Fatalf("control character left in text %q", sp.Text)
				}
			}
		}
	}
}
