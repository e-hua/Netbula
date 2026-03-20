package store

type InMemoryStore[T any] struct {
	Db map[string]*T
}

func NewInMemoryStore[T any]() *InMemoryStore[T] {
	return &InMemoryStore[T]{
		Db: make(map[string]*T),
	}
}

func (i *InMemoryStore[T]) Put(key string, value *T) error {
	i.Db[key] = value
	return nil
}

func (i *InMemoryStore[T]) Get(key string) (*T, error) {
	t, ok := i.Db[key]
	if !ok {
		return nil, nil
	}

	return t, nil
}

func (i *InMemoryStore[T]) List() ([]*T, error) {
	elements := make([]*T, 0, len(i.Db))

	for _, currElem := range i.Db {
		elements = append(elements, currElem)
	}

	return elements, nil
}

func (i *InMemoryStore[T]) Entries() ([]Entry[T], error) {
	entries := make([]Entry[T], 0)

	for currKey, currElem := range i.Db {
		entries = append(entries, Entry[T]{Key: currKey, Value: currElem})
	}

	return entries, nil
}

func (i *InMemoryStore[T]) Count() (int, error) {
	return len(i.Db), nil
}
