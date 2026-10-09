package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// counters talks to the same site-counters API as the website, so SSH
// visits and claps add to the numbers shown on miro.zip.
type counters struct {
	api    string
	client http.Client
}

func newCounters(api string) *counters {
	return &counters{api: api, client: http.Client{Timeout: 5 * time.Second}}
}

type countsMsg struct {
	claps, visitors int
	err             error
}

func (c *counters) call(method, path string) (map[string]int, error) {
	req, err := http.NewRequest(method, c.api+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s %s: %s", method, path, resp.Status)
	}
	var out map[string]int
	return out, json.NewDecoder(resp.Body).Decode(&out)
}

// visit registers this session as a visitor, then reads both counts.
func (c *counters) visit() tea.Cmd {
	return func() tea.Msg {
		if _, err := c.call(http.MethodPost, "/visit"); err != nil {
			return countsMsg{err: err}
		}
		n, err := c.call(http.MethodGet, "/counts")
		return countsMsg{claps: n["claps"], visitors: n["visitors"], err: err}
	}
}

type clapMsg struct {
	claps int
	err   error
}

func (c *counters) clap() tea.Cmd {
	return func() tea.Msg {
		n, err := c.call(http.MethodPost, "/clap")
		return clapMsg{claps: n["claps"], err: err}
	}
}
