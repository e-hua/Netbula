package store

import (
	"encoding/json"

	bolt "go.etcd.io/bbolt"
)

type PersistentStore[T any] struct {
	Db         *bolt.DB
	BucketName string
}

func NewPersistentStore[T any](db *bolt.DB, bucketName string) (*PersistentStore[T], error) {
	// Need to create the bucket
	persistentDb := &PersistentStore[T]{
		Db:         db,
		BucketName: bucketName,
	}

	err := persistentDb.CreateBucket()

	return persistentDb, err
}

func (p *PersistentStore[T]) Put(key string, value *T) error {
	// A read operation
	return p.Db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(p.BucketName))

		valueInBytes, err := json.Marshal(value)
		if err != nil {
			return err
		}

		err = bucket.Put([]byte(key), valueInBytes)
		return err
	})
}

func (p *PersistentStore[T]) Get(key string) (*T, error) {
	var val T
	var isInBucket bool

	err := p.Db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(p.BucketName))
		valueInBytes := bucket.Get([]byte(key))

		// Key not in the bucket
		// Not returning an error
		if valueInBytes == nil {
			isInBucket = false
			return nil
		}

		// Key in the bucket
		isInBucket = true
		return json.Unmarshal(valueInBytes, &val)
	})

	if !isInBucket {
		return nil, nil
	}

	return &val, err
}

func (p *PersistentStore[T]) List() ([]*T, error) {
	valueCount, err := p.Count()
	if err != nil {
		return nil, err
	}

	var values []*T = make([]*T, 0, valueCount)

	err = p.Db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(p.BucketName))

		err := bucket.ForEach(func(k, valueInBytes []byte) error {
			var value T
			currErr := json.Unmarshal(valueInBytes, &value)
			values = append(values, &value)

			return currErr
		})

		return err
	})

	return values, err
}

func (p *PersistentStore[T]) Entries() ([]Entry[T], error) {
	valueCount, err := p.Count()
	if err != nil {
		return nil, err
	}

	var entries []Entry[T] = make([]Entry[T], 0, valueCount)

	err = p.Db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(p.BucketName))

		err := bucket.ForEach(func(k, valueInBytes []byte) error {
			var value T

			currErr := json.Unmarshal(valueInBytes, &value)
			entries = append(entries, Entry[T]{Key: string(k), Value: &value})

			return currErr
		})

		return err
	})

	return entries, err
}

func (p *PersistentStore[T]) Count() (int, error) {
	keyCount := 0

	// Read the number of key/value pairs in the bucket
	p.Db.View(func(tx *bolt.Tx) error {
		keyCount = tx.Bucket([]byte(p.BucketName)).Stats().KeyN
		return nil
	})

	return keyCount, nil
}

func (p *PersistentStore[T]) CreateBucket() error {
	return p.Db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(p.BucketName))
		return err
	})
}

func (p *PersistentStore[T]) Close() error {
	return p.Db.Close()
}
