package store

type Entry[T any] struct {
	Key   string
	Value *T
}

type Store[T any] interface {
	Put(key string, value *T) error
	// Return error if database is not working
	// Return nil pointer if the key does not exist in the bucket
	Get(key string) (*T, error)
	List() ([]*T, error)
	Entries() ([]Entry[T], error)
	Count() (int, error)
}
