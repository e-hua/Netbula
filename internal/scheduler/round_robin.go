package scheduler

import (
	"github.com/e-hua/netbula/internal/node"
	"github.com/e-hua/netbula/internal/task"
	"github.com/google/uuid"
)

type RoundRobin struct {
	Name          string
	LastWorkerIdx int
}

func (r *RoundRobin) SelectCandidateNodes(t task.Task, nodes []node.Node) []node.Node {
	return nodes
}

func (r *RoundRobin) Score(t task.Task, nodes []node.Node) map[uuid.UUID]float64 {
	nodeScores := make(map[uuid.UUID]float64)

	newWorkerIdx := (r.LastWorkerIdx + 1) % len(nodes)
	r.LastWorkerIdx = newWorkerIdx

	for idx, node := range nodes {
		if idx == newWorkerIdx {
			nodeScores[node.WorkerUuid] = 0.1
		} else {
			nodeScores[node.WorkerUuid] = 1.0
		}
	}

	return nodeScores
}

// Pick the node with lowest score
func (r *RoundRobin) Pick(scores map[uuid.UUID]float64, candidates []node.Node) *node.Node {
	var bestNode *node.Node
	var lowestScore float64

	for idx, node := range candidates {
		if idx == 0 {
			bestNode = &node
			lowestScore = scores[node.WorkerUuid]
		} else if scores[node.WorkerUuid] < lowestScore {
			bestNode = &node
			lowestScore = scores[node.WorkerUuid]
		}
	}

	return bestNode
}
