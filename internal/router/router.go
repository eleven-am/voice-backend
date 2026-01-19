package router

import (
	"context"
	"sort"
	"strings"
	"sync"
	"unicode"
)

type tokenWeight struct {
	agentID string
	weight  int
}

type SmartRouter struct {
	mu       sync.RWMutex
	health   map[string]HealthMetrics
	index    map[string][]tokenWeight
	agentIDs []string
}

func NewSmartRouter() *SmartRouter {
	return &SmartRouter{
		health: make(map[string]HealthMetrics),
		index:  make(map[string][]tokenWeight),
	}
}

func (r *SmartRouter) SetHealth(h map[string]HealthMetrics) {
	r.mu.Lock()
	r.health = h
	r.mu.Unlock()
}

func (r *SmartRouter) Index(agents []AgentInfo) {
	index := make(map[string][]tokenWeight)
	agentIDs := make([]string, 0, len(agents))

	for _, agent := range agents {
		agentIDs = append(agentIDs, agent.ID)

		for _, kw := range agent.Keywords {
			for _, tok := range tokenize(kw) {
				index[tok] = append(index[tok], tokenWeight{agentID: agent.ID, weight: 3})
			}
		}

		for _, cap := range agent.Capabilities {
			for _, tok := range tokenize(cap) {
				index[tok] = append(index[tok], tokenWeight{agentID: agent.ID, weight: 2})
			}
		}

		for _, tok := range tokenize(agent.Description) {
			index[tok] = append(index[tok], tokenWeight{agentID: agent.ID, weight: 1})
		}

		for _, ex := range agent.Examples {
			for _, tok := range tokenize(ex) {
				index[tok] = append(index[tok], tokenWeight{agentID: agent.ID, weight: 1})
			}
		}
	}

	r.mu.Lock()
	r.index = index
	r.agentIDs = agentIDs
	r.mu.Unlock()
}

func (r *SmartRouter) Route(_ context.Context, request string, agents []AgentInfo) []string {
	if len(agents) == 0 {
		return nil
	}

	if len(agents) == 1 {
		return []string{agents[0].ID}
	}

	r.mu.RLock()
	indexed := len(r.index) > 0
	r.mu.RUnlock()

	if !indexed {
		r.Index(agents)
	}

	if request == "" {
		return r.selectByHealth(agents)
	}

	tokens := tokenize(request)
	if len(tokens) == 0 {
		return r.selectByHealth(agents)
	}

	scores := r.scoreFromIndex(tokens, agents)
	if len(scores) == 0 {
		return r.selectByHealth(agents)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	sort.Slice(scores, func(i, j int) bool {
		if scores[i].score != scores[j].score {
			return scores[i].score > scores[j].score
		}
		return r.compareHealthLocked(scores[i].id, scores[j].id)
	})

	result := make([]string, len(scores))
	for i, m := range scores {
		result[i] = m.id
	}
	return result
}

type agentMatch struct {
	id    string
	score int
}

func (r *SmartRouter) scoreFromIndex(tokens []string, agents []AgentInfo) []agentMatch {
	validIDs := make(map[string]bool, len(agents))
	for _, a := range agents {
		validIDs[a.ID] = true
	}

	scores := make(map[string]int)

	r.mu.RLock()
	for _, tok := range tokens {
		if weights, ok := r.index[tok]; ok {
			for _, tw := range weights {
				if validIDs[tw.agentID] {
					scores[tw.agentID] += tw.weight
				}
			}
		}
	}
	r.mu.RUnlock()

	matches := make([]agentMatch, 0, len(scores))
	for id, score := range scores {
		if score > 0 {
			matches = append(matches, agentMatch{id: id, score: score})
		}
	}
	return matches
}

func (r *SmartRouter) selectByHealth(agents []AgentInfo) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if best := selectBestByHealth(agents, r.health); best != "" {
		return []string{best}
	}
	return nil
}

func (r *SmartRouter) compareHealthLocked(a, b string) bool {
	ha, okA := r.health[a]
	hb, okB := r.health[b]

	if !okA && !okB {
		return false
	}
	if !okA {
		return false
	}
	if !okB {
		return true
	}

	if ha.Healthy != hb.Healthy {
		return ha.Healthy
	}

	return ha.LatencyMs < hb.LatencyMs
}

func tokenize(s string) []string {
	s = strings.ToLower(s)
	var words []string
	var current strings.Builder

	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else if current.Len() > 0 {
			word := current.String()
			if len(word) >= 2 {
				words = append(words, word)
			}
			current.Reset()
		}
	}

	if current.Len() > 0 {
		word := current.String()
		if len(word) >= 2 {
			words = append(words, word)
		}
	}

	return words
}
