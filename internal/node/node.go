package node

// The physical aspect of the Worker
type Node struct {
	Name string
	Cores int 
	Memory int 
	MemoryAllocated int 
	Disk int 
	DiskAllocated int 
	Role string
	TaskCount int 
}
