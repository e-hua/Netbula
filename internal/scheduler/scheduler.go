package scheduler

type Scheduler interface {
	SelectCandidiateNodes()
	Score()
	Pick()
}