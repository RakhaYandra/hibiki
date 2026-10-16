package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/RakhaYandra/hibiki/internal/timeline"
)

type queryDoneMsg struct {
	token int
	res   timeline.Result
	err   error
}

func (m *Model) startQuery(f timeline.Filter, limit int, after string) tea.Cmd {
	m.queryToken++
	token := m.queryToken
	if m.cancel != nil {
		m.cancel()
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	m.filter = f
	m.limit = limit
	m.loading = true
	m.err = nil
	return func() tea.Msg {
		res, err := timeline.Run(ctx, m.deps.Sources, f, limit, after)
		return queryDoneMsg{token: token, res: res, err: err}
	}
}

func (m *Model) applyQueryDone(msg queryDoneMsg) tea.Cmd {
	if msg.token != m.queryToken {
		return nil
	}
	m.loading = false
	if msg.err != nil {
		m.events = nil
		m.err = msg.err
		return nil
	}
	m.err = nil
	m.saveWarning = ""
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
