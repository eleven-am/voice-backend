package voicesession

import "sync"

type Arbiter struct {
	mu      sync.Mutex
	active  map[string]bool
	winner  string
	started bool
}

func NewArbiter() *Arbiter {
	return &Arbiter{
		active: make(map[string]bool),
	}
}

func (a *Arbiter) Start(agentIDs []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.active = make(map[string]bool, len(agentIDs))
	for _, id := range agentIDs {
		a.active[id] = true
	}
	a.winner = ""
	a.started = true
}

func (a *Arbiter) Decide(agentID string) (winner string, isNew bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.started || !a.active[agentID] {
		return a.winner, false
	}
	if a.winner != "" {
		return a.winner, false
	}
	a.winner = agentID
	return agentID, true
}

func (a *Arbiter) Losers() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.winner == "" {
		return nil
	}
	losers := make([]string, 0, len(a.active))
	for id := range a.active {
		if id != a.winner {
			losers = append(losers, id)
		}
	}
	return losers
}

func (a *Arbiter) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.active = make(map[string]bool)
	a.winner = ""
	a.started = false
}

func (a *Arbiter) Winner() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.winner
}
