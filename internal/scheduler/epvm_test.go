package scheduler

import (
	"math"
	"testing"

	"github.com/e-hua/netbula/internal/node"
	"github.com/google/uuid"
)

func TestEpvm_SelectCandidateNodes(t *testing.T) {
	var s Scheduler = &Epvm{}

	// Case 1: All nodes have enough disk space
	nodes := []*node.Node{
		mockNode("Node1", 4, 1.0, 8000, 50, 100, 10), // 100 * (1-0.10) = 90 available
		mockNode("Node2", 4, 1.0, 8000, 50, 200, 50), // 200 * (1-0.50) = 100 available
	}
	tsk := mockTask(1.0, 1000, 50) // needs 50 disk

	candidates := s.SelectCandidateNodes(tsk, nodes)
	if len(candidates) != 2 {
		t.Errorf("Expected 2 candidates, got %d", len(candidates))
	}

	// Case 2: Some nodes have insufficient space
	task2 := mockTask(1.0, 1000, 95) // needs 95 disk. Node1 only has 90. Node2 has 100.
	candidates2 := s.SelectCandidateNodes(task2, nodes)
	if len(candidates2) != 1 || candidates2[0].Name != "Node2" {
		t.Errorf("Expected 1 candidate (Node2), got %d", len(candidates2))
	}

	// Case 3: No nodes have enough space
	task3 := mockTask(1.0, 1000, 150)
	candidates3 := s.SelectCandidateNodes(task3, nodes)
	if len(candidates3) != 0 {
		t.Errorf("Expected 0 candidates, got %d", len(candidates3))
	}
}

func TestEpvm_Score(t *testing.T) {
	var s Scheduler = &Epvm{}

	n1 := mockNode("Node1", 4, 2.0, 8000, 25, 100, 10)
	// cpuLoad = 2.0 / 4 = 0.5
	// memoryUsagePercent = 25 / 100 = 0.25 (in Epvm score it divides by 100)
	nodes := []*node.Node{n1}

	tsk := mockTask(2.0, 4000, 50)
	// tsk.Cpu = 2.0. tsk.Memory = 4000
	// predictedCpuLoad = 0.5 + 2.0/4 = 1.0
	// predictedMemPct = 0.25 + 4000/8000 = 0.75

	scores := s.Score(tsk, nodes)
	if len(scores) != 1 {
		t.Fatalf("Expected 1 score, got %d", len(scores))
	}

	expectedMemCost := math.Pow(LIEB, 0.75) - math.Pow(LIEB, 0.25)
	expectedCpuCost := math.Pow(LIEB, 1.0) - math.Pow(LIEB, 0.5)
	expectedCost := expectedMemCost + expectedCpuCost

	// Using a small epsilon to account for floating point inaccuracies
	if math.Abs(scores[n1.WorkerUuid]-expectedCost) > 1e-9 {
		t.Errorf("Expected score %f, got %f", expectedCost, scores[n1.WorkerUuid])
	}
}

// One with lowest cost should be picked
func TestEpvm_Pick(t *testing.T) {
	var s Scheduler = &Epvm{}

	n1 := mockNode("Node1", 4, 1.0, 8000, 50, 100, 10)
	n2 := mockNode("Node2", 4, 1.0, 8000, 50, 100, 10)
	n3 := mockNode("Node3", 4, 1.0, 8000, 50, 100, 10)

	scores := map[uuid.UUID]float64{
		n1.WorkerUuid: 2.5,
		n2.WorkerUuid: 1.2,
		n3.WorkerUuid: 3.8,
	}

	best := s.Pick(scores, []*node.Node{n1, n2, n3})
	if best == nil || best.Name != "Node2" {
		t.Errorf("Expected to pick Node2, got %v", best)
	}

	// Empty candidate list
	bestEmpty := s.Pick(scores, []*node.Node{})
	if bestEmpty != nil {
		t.Errorf("Expected `nil` when pick from empty list, got %v", bestEmpty)
	}
}
