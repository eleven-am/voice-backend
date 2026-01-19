package voicesession

import (
	"sync"
	"testing"
)

func TestNewArbiter(t *testing.T) {
	arbiter := NewArbiter()
	if arbiter == nil {
		t.Fatal("NewArbiter should not return nil")
	}
	if arbiter.Winner() != "" {
		t.Errorf("initial winner should be empty, got %s", arbiter.Winner())
	}
}

func TestArbiter_Start(t *testing.T) {
	arbiter := NewArbiter()
	agents := []string{"agent1", "agent2", "agent3"}
	arbiter.Start(agents)
	if !arbiter.started {
		t.Error("started should be true after Start")
	}
	if len(arbiter.active) != 3 {
		t.Errorf("expected 3 active agents, got %d", len(arbiter.active))
	}
	for _, id := range agents {
		if !arbiter.active[id] {
			t.Errorf("agent %s should be active", id)
		}
	}
}

func TestArbiter_Start_ClearsPrevious(t *testing.T) {
	arbiter := NewArbiter()
	arbiter.Start([]string{"agent1", "agent2"})
	arbiter.Decide("agent1")
	arbiter.Start([]string{"agent3", "agent4"})
	if arbiter.Winner() != "" {
		t.Error("winner should be cleared after new Start")
	}
	if arbiter.active["agent1"] {
		t.Error("old agents should be cleared")
	}
	if !arbiter.active["agent3"] {
		t.Error("new agents should be active")
	}
}

func TestArbiter_Decide_FirstWins(t *testing.T) {
	arbiter := NewArbiter()
	arbiter.Start([]string{"agent1", "agent2", "agent3"})
	winner, isNew := arbiter.Decide("agent2")
	if winner != "agent2" {
		t.Errorf("expected winner agent2, got %s", winner)
	}
	if !isNew {
		t.Error("first decision should return isNew=true")
	}
	if arbiter.Winner() != "agent2" {
		t.Errorf("Winner() should return agent2, got %s", arbiter.Winner())
	}
}

func TestArbiter_Decide_SubsequentLose(t *testing.T) {
	arbiter := NewArbiter()
	arbiter.Start([]string{"agent1", "agent2", "agent3"})
	arbiter.Decide("agent1")
	winner, isNew := arbiter.Decide("agent2")
	if winner != "agent1" {
		t.Errorf("expected winner to remain agent1, got %s", winner)
	}
	if isNew {
		t.Error("subsequent decision should return isNew=false")
	}
}

func TestArbiter_Decide_NotStarted(t *testing.T) {
	arbiter := NewArbiter()
	winner, isNew := arbiter.Decide("agent1")
	if winner != "" {
		t.Errorf("expected empty winner when not started, got %s", winner)
	}
	if isNew {
		t.Error("should return isNew=false when not started")
	}
}

func TestArbiter_Decide_UnknownAgent(t *testing.T) {
	arbiter := NewArbiter()
	arbiter.Start([]string{"agent1", "agent2"})
	winner, isNew := arbiter.Decide("unknown")
	if winner != "" {
		t.Errorf("expected empty winner for unknown agent, got %s", winner)
	}
	if isNew {
		t.Error("should return isNew=false for unknown agent")
	}
}

func TestArbiter_Losers(t *testing.T) {
	arbiter := NewArbiter()
	arbiter.Start([]string{"agent1", "agent2", "agent3"})
	arbiter.Decide("agent2")
	losers := arbiter.Losers()
	if len(losers) != 2 {
		t.Fatalf("expected 2 losers, got %d", len(losers))
	}
	loserMap := make(map[string]bool)
	for _, l := range losers {
		loserMap[l] = true
	}
	if !loserMap["agent1"] || !loserMap["agent3"] {
		t.Errorf("expected agent1 and agent3 as losers, got %v", losers)
	}
	if loserMap["agent2"] {
		t.Error("winner should not be in losers")
	}
}

func TestArbiter_Losers_NoWinner(t *testing.T) {
	arbiter := NewArbiter()
	arbiter.Start([]string{"agent1", "agent2"})
	losers := arbiter.Losers()
	if losers != nil {
		t.Errorf("expected nil losers when no winner, got %v", losers)
	}
}

func TestArbiter_Reset(t *testing.T) {
	arbiter := NewArbiter()
	arbiter.Start([]string{"agent1", "agent2"})
	arbiter.Decide("agent1")
	arbiter.Reset()
	if arbiter.Winner() != "" {
		t.Error("winner should be empty after reset")
	}
	if arbiter.started {
		t.Error("started should be false after reset")
	}
	if len(arbiter.active) != 0 {
		t.Error("active should be empty after reset")
	}
}

func TestArbiter_Winner_EmptyInitially(t *testing.T) {
	arbiter := NewArbiter()
	if arbiter.Winner() != "" {
		t.Errorf("initial winner should be empty, got %s", arbiter.Winner())
	}
}

func TestArbiter_Concurrent(t *testing.T) {
	arbiter := NewArbiter()
	agents := []string{"agent1", "agent2", "agent3", "agent4", "agent5"}
	arbiter.Start(agents)
	var wg sync.WaitGroup
	winners := make(chan string, len(agents))
	for _, agent := range agents {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			winner, isNew := arbiter.Decide(id)
			if isNew {
				winners <- winner
			}
		}(agent)
	}
	wg.Wait()
	close(winners)
	winCount := 0
	for range winners {
		winCount++
	}
	if winCount != 1 {
		t.Errorf("expected exactly 1 winner, got %d", winCount)
	}
	finalWinner := arbiter.Winner()
	if finalWinner == "" {
		t.Error("should have a winner after concurrent decisions")
	}
	found := false
	for _, agent := range agents {
		if agent == finalWinner {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("winner %s should be one of the agents", finalWinner)
	}
}

func TestArbiter_MultipleRounds(t *testing.T) {
	arbiter := NewArbiter()
	arbiter.Start([]string{"agent1", "agent2"})
	winner1, _ := arbiter.Decide("agent1")
	if winner1 != "agent1" {
		t.Errorf("round 1: expected agent1, got %s", winner1)
	}
	arbiter.Reset()
	arbiter.Start([]string{"agent3", "agent4"})
	winner2, isNew := arbiter.Decide("agent4")
	if winner2 != "agent4" {
		t.Errorf("round 2: expected agent4, got %s", winner2)
	}
	if !isNew {
		t.Error("round 2: should be new winner")
	}
}
