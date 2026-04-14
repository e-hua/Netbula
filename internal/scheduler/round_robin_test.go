package scheduler

import (
	"testing"

	"github.com/e-hua/netbula/internal/node"
	"github.com/google/uuid"
)

func TestRoundRobin_SelectCandidateNodes(t *testing.T) {
	var s Scheduler = &RoundRobin{}

	nodes := []node.Node{
		mockNode("Node1", 4, 1.0, 4000, 50, 100, 10),
		mockNode("Node2", 4, 1.0, 4000, 50, 100, 10),
	}
	tsk := mockTask(1.0, 1000, 50)

	candidates := s.SelectCandidateNodes(tsk, nodes)
	if len(candidates) != len(nodes) {
		t.Errorf("Expected %d candidates, got %d", len(nodes), len(candidates))
	}
}

func TestRoundRobin_Score(t *testing.T) {
	rr := &RoundRobin{LastWorkerIdx: 0}
	var s Scheduler = rr

	nodes := []node.Node{
		mockNode("Node1", 4, 1.0, 4000, 50, 100, 10),
		mockNode("Node2", 4, 1.0, 4000, 50, 100, 10),
		mockNode("Node3", 4, 1.0, 4000, 50, 100, 10),
	}
	tsk := mockTask(1.0, 1000, 50)

	// First pass: LastWorkerIdx is 0. Next should be 1.
	scores1 := s.Score(tsk, nodes)
	if rr.LastWorkerIdx != 1 {
		t.Errorf("Expected LastWorkerIdx to be 1, got %d", rr.LastWorkerIdx)
	}
	if scores1[nodes[1].WorkerUuid] != 0.1 || scores1[nodes[0].WorkerUuid] != 1.0 || scores1[nodes[2].WorkerUuid] != 1.0 {
		t.Errorf("Incorrect scores after first pass: %v", scores1)
	}

	// Second pass: Next should be 2.
	scores2 := s.Score(tsk, nodes)
	if rr.LastWorkerIdx != 2 {
		t.Errorf("Expected LastWorkerIdx to be 2, got %d", rr.LastWorkerIdx)
	}
	if scores2[nodes[2].WorkerUuid] != 0.1 {
		t.Errorf("Incorrect scores after second pass: %v", scores2)
	}

	// Third pass (Wrap-around): Next should be 0.
	scores3 := s.Score(tsk, nodes)
	if rr.LastWorkerIdx != 0 {
		t.Errorf("Expected LastWorkerIdx to be 0, got %d", rr.LastWorkerIdx)
	}
	if scores3[nodes[0].WorkerUuid] != 0.1 {
		t.Errorf("Incorrect scores after wrap-around: %v", scores3)
	}
}

// One with lowest cost should be picked
func TestRoundRobin_Pick(t *testing.T) {
	var s Scheduler = &RoundRobin{}

	n1 := mockNode("Node1", 4, 1.0, 4000, 50, 100, 10)
	n2 := mockNode("Node2", 4, 1.0, 4000, 50, 100, 10)
	n3 := mockNode("Node3", 4, 1.0, 4000, 50, 100, 10)

	scores := map[uuid.UUID]float64{
		n1.WorkerUuid: 1.0,
		n2.WorkerUuid: 0.1,
		n3.WorkerUuid: 1.0,
	}

	best := s.Pick(scores, []node.Node{n1, n2, n3})
	if best == nil || best.Name != "Node2" {
		t.Errorf("Expected to pick Node2, got %v", best)
	}

	bestEmpty := s.Pick(scores, []node.Node{})
	if bestEmpty != nil {
		t.Errorf("Expected nil when pick from empty list, got %v", bestEmpty)
	}
}
