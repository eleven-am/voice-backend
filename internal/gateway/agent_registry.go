package gateway

import (
	"errors"
	"sync"
)

var ErrAgentAlreadyConnected = errors.New("agent already connected")

type AgentRegistry struct {
	conns map[string]AgentConnection
	mu    sync.RWMutex
}

func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		conns: make(map[string]AgentConnection),
	}
}

func (r *AgentRegistry) Register(conn AgentConnection) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	agentID := conn.AgentID()
	if existing, ok := r.conns[agentID]; ok && existing.IsOnline() {
		return ErrAgentAlreadyConnected
	}

	r.conns[agentID] = conn
	return nil
}

func (r *AgentRegistry) Unregister(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.conns, agentID)
}

func (r *AgentRegistry) Get(agentID string) (AgentConnection, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	conn, ok := r.conns[agentID]
	if !ok || !conn.IsOnline() {
		return nil, false
	}
	return conn, true
}

func (r *AgentRegistry) GetSSE(agentID string) (*SSEAgentConn, bool) {
	conn, ok := r.Get(agentID)
	if !ok {
		return nil, false
	}

	sseConn, ok := conn.(*SSEAgentConn)
	return sseConn, ok
}

func (r *AgentRegistry) IsOnline(agentID string) bool {
	_, ok := r.Get(agentID)
	return ok
}

func (r *AgentRegistry) List() []AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var agents []AgentInfo
	for agentID, conn := range r.conns {
		if conn.IsOnline() {
			agents = append(agents, AgentInfo{
				ID:     agentID,
				Online: true,
			})
		}
	}
	return agents
}

func (r *AgentRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, conn := range r.conns {
		if conn.IsOnline() {
			count++
		}
	}
	return count
}
