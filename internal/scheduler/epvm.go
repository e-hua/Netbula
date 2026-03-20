package scheduler

import (
	"math"

	"github.com/e-hua/netbula/internal/node"
	"github.com/e-hua/netbula/internal/task"
	"github.com/google/uuid"
)

const (
	// LIEB square ice constant
	// https://en.wikipedia.org/wiki/Lieb%27s_square_ice_constant
	LIEB = 1.53960071783900203869
)

type Epvm struct {
}

func checkDisk(t task.Task, diskAvailable int) bool {
	return t.Disk <= diskAvailable
}

// Choosing the nodes with enough disk space
func (e *Epvm) SelectCandidateNodes(t task.Task, nodes []*node.Node) []*node.Node {
	var candidates []*node.Node

	for nodeIdx := range nodes {
		currNode := nodes[nodeIdx]
		diskAvailable := (1 - 0.01*currNode.DiskAllocatedPercent) * float64(currNode.Disk)

		if checkDisk(t, int(diskAvailable)) {
			candidates = append(candidates, currNode)
		}
	}

	return candidates
}

// Score the nodes using the E-PVM algorithm
// Link to paper: https://mosix.cs.huji.ac.il/pub/ocja.pdf

func (e *Epvm) Score(t task.Task, nodes []*node.Node) map[uuid.UUID]float64 {
	nodeScores := make(map[uuid.UUID]float64)

	for _, currNode := range nodes {

		cpuLoad := currNode.CpuAverageLoad / float64(currNode.Cores)
		predictedCpuLoad := cpuLoad + t.Cpu/float64(currNode.Cores)

		memoryUsagePercent := currNode.MemoryAllocatedPercent / 100
		predictedMemoryUsagePercent := memoryUsagePercent + (float64(t.Memory) / float64(currNode.Memory))

		memCost := math.Pow(LIEB, predictedMemoryUsagePercent) - math.Pow(LIEB, memoryUsagePercent)
		cpuCost := math.Pow(LIEB, predictedCpuLoad) - math.Pow(LIEB, cpuLoad)

		nodeScores[currNode.WorkerUuid] = memCost + cpuCost
	}

	return nodeScores
}

// Pick the node with lowest score (price in this case)
func (e *Epvm) Pick(scores map[uuid.UUID]float64, candidates []*node.Node) *node.Node {
	var minCost float64
	var bestNode *node.Node

	for idx, node := range candidates {
		if idx == 0 {
			minCost = scores[node.WorkerUuid]
			bestNode = node
			continue
		}

		if scores[node.WorkerUuid] < minCost {
			minCost = scores[node.WorkerUuid]
			bestNode = node
		}
	}

	return bestNode
}
