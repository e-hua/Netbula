package scheduler

import (
	"github.com/e-hua/netbula/internal/node"
	"github.com/e-hua/netbula/internal/task"
	"github.com/google/uuid"
)

type Scheduler interface {
	SelectCandidateNodes(t task.Task, nodes []*node.Node) []*node.Node
	Score(t task.Task, nodes []*node.Node) map[uuid.UUID]float64
	Pick(scores map[uuid.UUID]float64, candidates []*node.Node) *node.Node
}
