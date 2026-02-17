package scheduler

import (
	"github.com/e-hua/netbula/internal/node"
	"github.com/e-hua/netbula/internal/task"
)

type Scheduler interface {
	SelectCandidateNodes(t task.Task, nodes []*node.Node) []*node.Node
	Score(t task.Task, nodes []*node.Node) map[string]float64
	Pick(scores map[string]float64, candidates []*node.Node) *node.Node
}

