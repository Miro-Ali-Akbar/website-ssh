package main

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

// Span is a run of paragraph text. Link is -1 for plain text, otherwise an
// index into the owning Section's Links.
type Span struct {
	Text string
	Link int
}

type Link struct {
	Text string
	URL  string
}

type Section struct {
	Title      string
	Paragraphs [][]Span
	Links      []Link
}

// snapshot holds copies of the site's HTML, used when the live site can't
// be reached. Refresh them with: cp ../website/{index,projects}.html snapshot/
//
//go:embed snapshot/*.html
var snapshot embed.FS

// store keeps the latest parsed content and swaps it in on refresh, so the
// terminal site always follows the real one without a redeploy.
type store struct {
	site string
	mu   sync.RWMutex
	data []Section
}

func newStore(site string) *store {
	st := &store{site: site}
	if err := st.refresh(); err != nil {
		log.Println("live site unavailable, using snapshot:", err)
		idx, _ := snapshot.ReadFile("snapshot/index.html")
		proj, _ := snapshot.ReadFile("snapshot/projects.html")
		data, err := parseSite(idx, proj)
		if err != nil {
			log.Fatalln("snapshot unreadable:", err)
		}
		st.data = data
	}
	return st
}

func (st *store) get() []Section {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.data
}

func (st *store) refreshLoop(every time.Duration) {
	for range time.Tick(every) {
		if err := st.refresh(); err != nil {
			log.Println("refresh failed, keeping old content:", err)
		}
	}
}

func (st *store) refresh() error {
	idx, err := fetch(st.site + "/index.html")
	if err != nil {
		return err
	}
	proj, err := fetch(st.site + "/projects.html")
	if err != nil {
		return err
	}
	data, err := parseSite(idx, proj)
	if err != nil {
		return err
	}
	st.mu.Lock()
	st.data = data
	st.mu.Unlock()
	return nil
}

func fetch(url string) ([]byte, error) {
	c := http.Client{Timeout: 10 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

// parseSite turns the website's HTML into sections. index.html provides
// the text sections and social links, projects.html the projects.
func parseSite(indexHTML, projectsHTML []byte) ([]Section, error) {
	idx, err := html.Parse(bytes.NewReader(indexHTML))
	if err != nil {
		return nil, err
	}
	proj, err := html.Parse(bytes.NewReader(projectsHTML))
	if err != nil {
		return nil, err
	}

	var p parser
	if center := find(idx, func(n *html.Node) bool { return hasClass(n, "floating-box") && hasClass(n, "center") }); center != nil {
		p.blocks(center)
	}
	if content := find(proj, func(n *html.Node) bool { return attr(n, "id") == "projects-content" }); content != nil {
		p.blocks(content)
	}

	links := Section{Title: "Links"}
	addLink := func(text, url string) {
		links.Links = append(links.Links, Link{text, url})
		links.Paragraphs = append(links.Paragraphs, []Span{{text, len(links.Links) - 1}})
	}
	walk(idx, func(n *html.Node) {
		if n.Data == "a" && hasClass(n, "social-item") {
			addLink(collapse(text(n)), attr(n, "href"))
		}
	})
	addLink("Website", "https://miro.zip")
	p.sections = append(p.sections, links)

	if len(p.sections) < 2 {
		return nil, fmt.Errorf("no sections found")
	}
	return p.sections, nil
}

type parser struct {
	sections []Section
}

func (p *parser) current() *Section {
	if len(p.sections) == 0 {
		p.sections = append(p.sections, Section{Title: "Home"})
	}
	return &p.sections[len(p.sections)-1]
}

// blocks walks block-level children: headings with an id start a new
// section, paragraphs and small headings become paragraphs, divs recurse.
func (p *parser) blocks(n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode {
			continue
		}
		switch c.Data {
		case "h1", "h2":
			if attr(c, "id") != "" {
				p.sections = append(p.sections, Section{Title: collapse(text(c))})
			}
		case "p", "h3":
			p.paragraph(c)
		case "div":
			p.blocks(c)
		}
	}
}

func (p *parser) paragraph(n *html.Node) {
	sec := p.current()
	var spans []Span
	var inline func(*html.Node)
	inline = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			switch {
			case c.Type == html.TextNode:
				spans = append(spans, Span{c.Data, -1})
			case c.Data == "a":
				sec.Links = append(sec.Links, Link{collapse(text(c)), attr(c, "href")})
				spans = append(spans, Span{collapse(text(c)), len(sec.Links) - 1})
			case c.Data == "br":
				spans = append(spans, Span{"\n", -1})
			case hasClass(c, "wave"):
				// One letter per span on the site, glue them back together.
				spans = append(spans, Span{strings.Join(strings.Fields(text(c)), ""), -1})
			default:
				inline(c)
			}
		}
	}
	inline(n)

	// Collapse whitespace the way a browser would, across span borders.
	out := spans[:0]
	prevSpace := true
	for _, s := range spans {
		if s.Text == "\n" {
			if n := len(out); n > 0 {
				out[n-1].Text = strings.TrimRight(out[n-1].Text, " ")
			}
			out = append(out, s)
			prevSpace = true
			continue
		}
		t := space.ReplaceAllString(s.Text, " ")
		if prevSpace {
			t = strings.TrimLeft(t, " ")
		}
		if t == "" {
			continue
		}
		prevSpace = strings.HasSuffix(t, " ")
		out = append(out, Span{t, s.Link})
	}
	if len(out) > 0 {
		last := &out[len(out)-1]
		last.Text = strings.TrimRight(last.Text, " ")
		sec.Paragraphs = append(sec.Paragraphs, out)
	}
}

var space = regexp.MustCompile(`\s+`)

func collapse(s string) string { return strings.TrimSpace(space.ReplaceAllString(s, " ")) }

func text(n *html.Node) string {
	var b strings.Builder
	walk(n, func(c *html.Node) {
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
		}
	})
	return b.String()
}

func walk(n *html.Node, f func(*html.Node)) {
	f(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, f)
	}
}

func find(n *html.Node, match func(*html.Node) bool) *html.Node {
	if n.Type == html.ElementNode && match(n) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if f := find(c, match); f != nil {
			return f
		}
	}
	return nil
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasClass(n *html.Node, class string) bool {
	for _, c := range strings.Fields(attr(n, "class")) {
		if c == class {
			return true
		}
	}
	return false
}
