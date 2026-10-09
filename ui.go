package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Colors from the website's style.css.
const (
	colorFocus = lipgloss.Color("#ffc480")
	colorRed   = lipgloss.Color("#fe4a60")
	colorBlack = lipgloss.Color("#111827")
	colorDim   = lipgloss.Color("#8a8a8a")
)

const (
	navWidth     = 18
	contentWidth = 76
)

type pane int

const (
	navPane pane = iota
	contentPane
)

type model struct {
	sections []Section
	out      io.Writer // the SSH session, for clipboard escapes
	counters *counters // nil when no counter API is configured

	page   int // section highlighted in the nav and shown in the content pane
	link   int // selected link in the shown section
	scroll int
	focus  pane
	onClap bool // nav cursor is on the clap button below the sections
	status string

	claps, visitors int
	countsOK        bool
	clapped         bool

	width, height int
	st            styles

	// Set while rendering, so key handling can stop scrolling at the end.
	maxScroll *int
}

type styles struct {
	box, boxFocus, title, navItem, navCursor, navPage, text, link, linkSel, help, status, digits lipgloss.Style
}

func newModel(sections []Section, c *counters, out io.Writer, w, h int, r *lipgloss.Renderer) model {
	box := r.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorDim).Padding(0, 1)
	return model{
		sections:  sections,
		out:       out,
		counters:  c,
		width:     w,
		height:    h,
		maxScroll: new(int),
		st: styles{
			box:       box,
			boxFocus:  box.BorderForeground(colorFocus),
			title:     r.NewStyle().Bold(true).Foreground(colorRed),
			navItem:   r.NewStyle(),
			navCursor: r.NewStyle().Background(colorFocus).Foreground(colorBlack).Bold(true),
			navPage:   r.NewStyle().Foreground(colorFocus).Bold(true),
			text:      r.NewStyle(),
			link:      r.NewStyle().Foreground(colorFocus).Underline(true),
			linkSel:   r.NewStyle().Background(colorFocus).Foreground(colorBlack).Bold(true),
			help:      r.NewStyle().Foreground(colorDim),
			status:    r.NewStyle().Foreground(colorFocus),
			digits:    r.NewStyle().Background(colorBlack).Foreground(colorFocus).Bold(true).Padding(0, 1),
		},
	}
}

func (m model) Init() tea.Cmd {
	if m.counters == nil {
		return nil
	}
	return m.counters.visit()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case countsMsg:
		m.countsOK = msg.err == nil
		m.claps, m.visitors = msg.claps, msg.visitors
	case clapMsg:
		if msg.err != nil {
			m.status = "Couldn't send the clap, try again later"
			m.clapped = false
		} else {
			m.claps = msg.claps
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			m.move(-1)
		case "down", "j":
			m.move(1)
		case "left", "h":
			m.focus = navPane
		case "right", "l":
			m.focus = contentPane
		case "tab":
			m.focus = 1 - m.focus
		case "enter", " ":
			return m, m.activate()
		case "c":
			return m, m.clap()
		}
	}
	return m, nil
}

func (m *model) move(d int) {
	m.status = ""
	if m.focus == navPane {
		// The clap button sits below the last section.
		if m.onClap {
			m.onClap = d > 0
			return
		}
		if m.page+d >= len(m.sections) {
			m.onClap = true
			return
		}
		// Moving through the nav opens each section right away.
		if p := clamp(m.page+d, 0, len(m.sections)-1); p != m.page {
			m.page, m.link, m.scroll = p, 0, 0
		}
		return
	}
	if links := m.sections[m.page].Links; len(links) > 0 {
		m.link = clamp(m.link+d, 0, len(links)-1)
	} else {
		m.scroll = clamp(m.scroll+d, 0, *m.maxScroll)
	}
}

func (m *model) activate() tea.Cmd {
	if m.focus == navPane {
		if m.onClap {
			return m.clap()
		}
		m.focus = contentPane
		m.status = ""
		return nil
	}
	links := m.sections[m.page].Links
	if len(links) == 0 {
		return nil
	}
	// A server can't open a browser on the visitor's machine. Instead the
	// URL is copied with OSC 52 and every link is an OSC 8 hyperlink, so
	// most terminals can ctrl/cmd+click it.
	url := links[m.link].URL
	fmt.Fprintf(m.out, "\x1b]52;c;%s\a", base64.StdEncoding.EncodeToString([]byte(url)))
	m.status = "Copied " + url
	return nil
}

// clap sends one clap per session, like the button on the website.
func (m *model) clap() tea.Cmd {
	switch {
	case m.counters == nil || !m.countsOK:
		m.status = "Counters are unavailable right now"
		return nil
	case m.clapped:
		m.status = "Already clapped, thank you!"
		return nil
	}
	m.clapped = true
	m.status = "Thanks for the clap!"
	return m.counters.clap()
}

func (m model) View() string {
	if m.width < navWidth+30 || m.height < 12 {
		return "Make the terminal a bit bigger to see miro.zip"
	}
	cw := min(contentWidth, m.width-navWidth-6)
	bodyHeight := m.height - 4 // borders plus the help line

	nav := m.navView(bodyHeight)
	content := m.contentView(cw-4, bodyHeight)

	navBox, contentBox := m.st.box, m.st.box
	if m.focus == navPane {
		navBox = m.st.boxFocus
	} else {
		contentBox = m.st.boxFocus
	}
	row := lipgloss.JoinHorizontal(lipgloss.Top,
		navBox.Width(navWidth).Height(bodyHeight).Render(nav),
		" ",
		contentBox.Width(cw).Height(bodyHeight).Render(content),
	)

	help := m.st.help.Render("↑↓/jk move · ←→/hl switch pane · enter/space open · c clap · q quit")
	if m.status != "" {
		help = m.st.status.Render(ansi.Truncate(m.status, navWidth+cw+5, "…"))
	}
	page := lipgloss.JoinVertical(lipgloss.Left, row, help)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, page)
}

func (m model) navView(height int) string {
	lines := []string{m.st.title.Render("miro.zip"), ""}
	for i, s := range m.sections {
		style := m.st.navItem
		if i == m.page && m.focus == navPane && !m.onClap {
			style = m.st.navCursor
		} else if i == m.page {
			style = m.st.navPage
		}
		lines = append(lines, style.Render(" "+s.Title+" "))
	}

	// Counters at the bottom, like the boxes under the website's nav.
	clapStyle := m.st.navItem
	if m.onClap && m.focus == navPane {
		clapStyle = m.st.navCursor
	}
	clapLabel := " Clap "
	if m.clapped {
		clapLabel = " Clapped "
	}
	counters := []string{
		clapStyle.Render(clapLabel),
		m.st.digits.Render(m.digits(m.claps)),
		"",
		m.st.navItem.Render(" Visitors "),
		m.st.digits.Render(m.digits(m.visitors)),
	}
	for len(lines)+len(counters) < height {
		lines = append(lines, "")
	}
	return strings.Join(append(lines, counters...), "\n")
}

func (m model) digits(n int) string {
	if !m.countsOK {
		return "-------"
	}
	return fmt.Sprintf("%07d", n%10000000)
}

func (m *model) contentView(width, height int) string {
	sec := m.sections[m.page]
	lines := []string{m.st.title.Render(sec.Title), ""}
	selLine := -1

	for _, para := range sec.Paragraphs {
		var b strings.Builder
		for _, s := range para {
			switch {
			case s.Link < 0:
				b.WriteString(m.st.text.Render(s.Text))
			case s.Link == m.link && m.focus == contentPane:
				selLine = len(lines)
				b.WriteString(hyperlink(sec.Links[s.Link].URL, m.st.linkSel.Render(s.Text)))
			default:
				b.WriteString(hyperlink(sec.Links[s.Link].URL, m.st.link.Render(s.Text)))
			}
		}
		wrapped := lipgloss.NewStyle().Width(width).Render(b.String())
		lines = append(lines, strings.Split(wrapped, "\n")...)
		lines = append(lines, "")
	}

	// Keep the selected link's paragraph on screen.
	if selLine >= 0 {
		if selLine < m.scroll {
			m.scroll = selLine
		} else if selLine >= m.scroll+height {
			m.scroll = selLine - height + 1
		}
	}
	*m.maxScroll = max(len(lines)-height, 0)
	m.scroll = clamp(m.scroll, 0, *m.maxScroll)
	end := min(m.scroll+height, len(lines))
	return strings.Join(lines[m.scroll:end], "\n")
}

func hyperlink(url, text string) string {
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

func clamp(v, lo, hi int) int { return max(lo, min(v, hi)) }
