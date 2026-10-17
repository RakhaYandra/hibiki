package tui

import (
	"context"
	"slices"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/RakhaYandra/hibiki/internal/timeline"
)

type queryDoneMsg struct {
	token int
	res   timeline.Result
	err   error
	page  bool
}

func (m *Model) startQuery(f timeline.Filter, limit int, after string) tea.Cmd {
	return m.startQueryInto(f, limit, after, false)
}

// loadPage asks for the next slice of the same window. It never touches
// m.limit: that is the size of a fresh window, and a refresh has to keep it.
func (m *Model) loadPage(after string) tea.Cmd {
	limit := m.deps.PageSize
	if limit <= 0 {
		limit = m.limit
	}
	return m.startQueryInto(m.filter, limit, after, true)
}

func (m *Model) maybeLoadMore() tea.Cmd {
	if !m.truncated || m.cursorNext == "" || m.loadingMore || len(m.events) == 0 {
		return nil
	}
	return m.loadPage(m.cursorNext)
}

func (m *Model) startQueryInto(f timeline.Filter, limit int, after string, page bool) tea.Cmd {
	m.queryToken++
	token := m.queryToken
	if m.run.cancel != nil {
		m.run.cancel()
	}
	ctx, cancel := context.WithCancel(m.ctx)
	finished := make(chan struct{})
	m.run.cancel = cancel
	m.run.done = finished
	if !page {
		m.filter = f
		m.limit = limit
		m.loading = true
	}
	m.loadingMore = page
	m.err = nil
	m.pageWarning = ""
	return func() tea.Msg {
		defer close(finished)
		res, err := timeline.Run(ctx, m.deps.Sources, f, limit, after)
		return queryDoneMsg{token: token, res: res, err: err, page: page}
	}
}

func (m *Model) applyQueryDone(msg queryDoneMsg) tea.Cmd {
	if msg.token != m.queryToken {
		return nil
	}
	m.loading = false
	m.loadingMore = false
	if msg.page {
		m.applyPageDone(msg)
		return nil
	}
	if msg.err != nil {
		m.events = nil
		m.err = msg.err
		return nil
	}
	m.err = nil
	m.saveWarning = ""
	m.duplicates = 0
	m.events = msg.res.Events
	m.truncated = msg.res.Truncated
	m.cursorNext = msg.res.NextCursor
	m.coveredSince = msg.res.CoveredSince
	m.coveredUntil = msg.res.CoveredUntil
	m.sources = msg.res.Sources
	m.limitations = msg.res.Limitations
	m.selected = 0
	m.offset = 0
	if m.deps.Save != nil {
		if _, err := m.deps.Save(msg.res); err != nil {
			m.saveWarning = err.Error()
		}
	}
	return nil
}

// A page failure is not a query failure: the events already on screen came from
// a query that worked, and replacing the list with an error screen would throw
// away what the person is reading.
func (m *Model) applyPageDone(msg queryDoneMsg) {
	if msg.err != nil {
		m.pageWarning = "next page failed: " + msg.err.Error()
		return
	}
	m.pageWarning = ""
	for _, ev := range msg.res.Events {
		if m.hasEvent(ev.ID) {
			m.duplicates++
			continue
		}
		m.events = append(m.events, ev)
	}
	m.truncated = msg.res.Truncated
	m.cursorNext = msg.res.NextCursor
	m.absorbCoverage(msg.res)
	m.absorbStatus(msg.res)
}

func (m *Model) hasEvent(id string) bool {
	for _, ev := range m.events {
		if ev.ID == id {
			return true
		}
	}
	return false
}

func (m *Model) absorbCoverage(res timeline.Result) {
	if !res.CoveredSince.IsZero() && (m.coveredSince.IsZero() || res.CoveredSince.Before(m.coveredSince)) {
		m.coveredSince = res.CoveredSince
	}
	if res.CoveredUntil.After(m.coveredUntil) {
		m.coveredUntil = res.CoveredUntil
	}
}

func (m *Model) absorbStatus(res timeline.Result) {
	if len(res.Sources) > 0 {
		m.sources = res.Sources
	}
	for _, limitation := range res.Limitations {
		if !slices.Contains(m.limitations, limitation) {
			m.limitations = append(m.limitations, limitation)
		}
	}
}
