package task

type State int 

const (
	Pending State = iota // State = 0
	Scheduled  // Scheduled = 1
	Running  // ...
	Completed
	Failed
)