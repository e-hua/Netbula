package store

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path"
	"testing"

	"github.com/google/go-cmp/cmp"
	bolt "go.etcd.io/bbolt"
)

const (
	testDbName                 = "test.db"
	testBucketName             = "testBucket"
	testDbFileMode os.FileMode = 0600
)

func createFile(filePath string) (err error) {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}

	defer func() {
		// We don't need to have this file open while using Bbolt
		err = errors.Join(err, file.Close())
	}()

	return err
}

func createTempDb(t *testing.T) (*bolt.DB, error) {
	tempDir := t.TempDir()
	tempDbFilePath := path.Join(tempDir, testDbName)

	err := createFile(tempDbFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create file in temporary directory %s: %w", tempDir, err)
	}

	db, err := bolt.Open(tempDbFilePath, testDbFileMode, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open Bbolt DB located at path %s: %w", tempDbFilePath, err)
	}

	return db, nil
}

func createTestPersistentStore[T any](t *testing.T) Store[T] {
	// Mark the function as helper,
	// so the error messAges can be linked back to the function calling it
	t.Helper()

	testDb, err := createTempDb(t)
	if err != nil {
		t.Fatalf("Failed to create temporary Bbolt DB for testing: %v", err)
	}

	testPersistentStore, err := NewPersistentStore[T](testDb, testBucketName)
	if err != nil {
		t.Fatalf("Failed to create temporary Bbolt DB for testing: %v", err)
	}

	t.Cleanup(func() {
		err = testPersistentStore.Close()
		if err != nil {
			t.Fatalf("Failed to close testPersistentStore in the cleanup function: %v", err)
		}
	})

	return testPersistentStore
}

// Testing PersistentStore.Put()
type testPutData[T any] struct {
	putKey string
	putVal T
}

func putValues[T any](t *testing.T, entries []testPutData[T], testStore Store[T]) {
	for _, entry := range entries {
		err := testStore.Put(entry.putKey, &entry.putVal)
		if err != nil {
			t.Errorf("Failed to put key-value pair %v into the persistent storage: %v", entry, err)
		}

		valFromStorage, err := testStore.Get(entry.putKey)
		if err != nil {
			t.Errorf("Failed to get value from persistent storage using key %s: %v", entry.putKey, err)
		}

		diff := cmp.Diff(entry.putVal, *valFromStorage)
		if diff != "" {
			t.Errorf("Value retrieved from persistent storage is different from the initial value: \n%s", diff)
		}
	}
}

func TestPersistenStorePut(t *testing.T) {
	t.Run("Integers", func(t *testing.T) {
		entries := []testPutData[int]{
			{putKey: "key-1", putVal: 1},
			{putKey: "*", putVal: 0},
			{putKey: "-", putVal: -1},
		}
		testPersistentStore := createTestPersistentStore[int](t)
		putValues(t, entries, testPersistentStore)
	})

	t.Run("Strings", func(t *testing.T) {
		entries := []testPutData[string]{
			{putKey: "key-1", putVal: ""},
			{putKey: " ", putVal: " "},
			{putKey: "key-2", putVal: "supercalifragilisticexpialidocious"},
		}
		testPersistentStore := createTestPersistentStore[string](t)
		putValues(t, entries, testPersistentStore)
	})

	t.Run("Structs", func(t *testing.T) {
		type testSimpleStruct struct {
			BoolVal   bool
			FloatVal  float64
			StringVal string
		}

		type testNestedStruct struct {
			Name         string
			Age          int
			SimpleStruct testSimpleStruct
		}

		entries := []testPutData[testNestedStruct]{
			{putKey: "key-1", putVal: testNestedStruct{}},
			{putKey: " ", putVal: testNestedStruct{Name: "", SimpleStruct: testSimpleStruct{StringVal: ""}}},
			{putKey: "key-2", putVal: testNestedStruct{Name: "", SimpleStruct: testSimpleStruct{FloatVal: 0.0000001}}},
			{putKey: "~", putVal: testNestedStruct{Name: "John", Age: 19, SimpleStruct: testSimpleStruct{BoolVal: true, FloatVal: -10.2, StringVal: "Some string"}}},
		}
		testPersistentStore := createTestPersistentStore[testNestedStruct](t)
		putValues(t, entries, testPersistentStore)
	})

}

// Testing PersistentStore.Get()
type testGetData[T any] struct {
	testPutData[T]
	expectedPutErrMsg string

	getKey            string
	expectedValPtr    *T
	expectedGetErrMsg string
}

func getValues[T any](t *testing.T, entries []testGetData[T], testStore Store[T]) {
	for _, entry := range entries {
		err := testStore.Put(entry.putKey, &entry.putVal)
		if err != nil {
			if err.Error() != entry.expectedPutErrMsg {
				t.Errorf("Error message %s thrown by storage while putting value is different from error expected %s", err.Error(), entry.expectedPutErrMsg)
			}
			continue
		}

		valFromStorage, err := testStore.Get(entry.getKey)
		if err != nil {
			if err.Error() != entry.expectedGetErrMsg {
				t.Errorf("Error message %s thrown by storage while getting value is different from error expected %s", err.Error(), entry.expectedGetErrMsg)
			}
			continue
		}

		if valFromStorage == nil || entry.expectedValPtr == nil {
			if !(valFromStorage == nil && entry.expectedValPtr == nil) {
				t.Errorf("Value %s retrieved from storage is different from value expected %s", valFromStorage, entry.expectedValPtr)
			}
			continue
		}

		diff := cmp.Diff(entry.putVal, *valFromStorage)
		if diff != "" {
			t.Errorf("Value retrieved from persistent storage is different from the expected value: \n%s", diff)
		}
	}
}

func TestPersistentStoreGet(t *testing.T) {
	t.Run("Error putting values in storage", func(t *testing.T) {
		testPersistentStore := createTestPersistentStore[float64](t)

		entries := []testGetData[float64]{
			{testPutData: testPutData[float64]{putKey: "key-1", putVal: math.NaN()}, expectedPutErrMsg: "json: unsupported value: NaN"},
		}
		getValues(t, entries, testPersistentStore)
	})

	t.Run("Key not in the storage", func(t *testing.T) {
		testPersistentStore := createTestPersistentStore[string](t)

		entries := []testGetData[string]{
			{testPutData: testPutData[string]{putKey: "key-1", putVal: "string_value"}, getKey: "", expectedValPtr: nil},
		}
		getValues(t, entries, testPersistentStore)
	})

}

// Compare the content of in-memory map and the returned list
func compareListWithInputEntries[T any](t *testing.T, inputEntries []testPutData[T], list []*T) {
	testMap := make(map[string]T)

	for _, entry := range inputEntries {
		testMap[entry.putKey] = entry.putVal
	}

	// Idea: Iterate over the map,
	// find one element in the map with the same go-cmp value as the current element in the map
	// And exclude the element already found in the list in the previous iteration

	// Error if any value left in the map/list
	storedListIndexSet := make(map[int]bool)

	for storedKey, storedVal := range testMap {
		var storedEntryFound bool
		for listIdx, listVal := range list {
			_, ok := storedListIndexSet[listIdx]
			if ok {
				continue
			}

			if cmp.Diff(storedVal, *listVal) == "" {
				storedListIndexSet[listIdx] = true
				storedEntryFound = true
			}
		}

		if !storedEntryFound {
			t.Errorf(
				"Key-value pair (with key: %s, value: %#v) in normal in-memory map is not stored in list %#v returned",
				storedKey,
				storedVal,
				list,
			)
		}
	}

	for listIdx, listVal := range list {
		_, ok := storedListIndexSet[listIdx]
		if !ok {
			t.Errorf(
				"Value returned from list method: %#v is not contained in normal in-memory map: %#v",
				listVal,
				testMap,
			)
		}
	}
}

var inputEntries = []testPutData[string]{
	{putKey: "key-1", putVal: "val-1"},
	{putKey: "key-2", putVal: "val-2"},
	{putKey: "key-3", putVal: "val-3"},
	{putKey: "key-1", putVal: "new-val-3"},
	{putKey: "key-2", putVal: ""},
}

// Testing PersistentStore.List()
func TestPersistentStoreList(t *testing.T) {
	testPersistentStore := createTestPersistentStore[string](t)
	putValues(t, inputEntries, testPersistentStore)

	listFromStore, err := testPersistentStore.List()
	if err != nil {
		t.Fatalf("Failed to get the list of stored values from the persistent storage: %v", err)
	}

	compareListWithInputEntries(t, inputEntries, listFromStore)
}

// Compare the content of in-memory map and the returned entries
func compareStoredEntriesWithInputEntries[T any](t *testing.T, inputEntries []testPutData[T], storedEntries []Entry[T]) {
	testMap := make(map[string]T)
	for _, inputEntry := range inputEntries {
		testMap[inputEntry.putKey] = inputEntry.putVal
	}

	storedEntryMap := make(map[string]T)
	for _, storedEntry := range storedEntries {
		key := storedEntry.Key
		val := storedEntry.Value

		_, ok := storedEntryMap[key]
		if ok {
			t.Errorf("Two entries returned by the persistent storage have the same key: %s", key)
		}

		storedEntryMap[key] = *val
	}

	for testMapKey, testMapVal := range testMap {
		storedEntryVal, ok := storedEntryMap[testMapKey]
		if !ok {
			t.Errorf(
				"The entries returned by the storage is supposed to contain an entry with key: [%s] and value: [%#v]",
				testMapKey,
				testMapVal,
			)
			continue
		}

		if diff := cmp.Diff(testMapVal, storedEntryVal); diff != "" {
			t.Errorf(
				"The entry returned by the storage with key: [%s] is supposed to have value: [%#v], instead of value [%#v]",
				testMapKey,
				testMapVal,
				storedEntryVal,
			)
		}
	}

	for storedKey, storedVal := range storedEntryMap {
		_, ok := testMap[storedKey]

		if !ok {
			t.Errorf(
				"The entries returned by the storage is not supposed to contain an entry with key: [%s] and value: [%#v]",
				storedKey,
				storedVal,
			)
		}
	}
}

// Testing PersistentStore.Entries()
func TestPersistentStoreEntries(t *testing.T) {
	testPersistentStore := createTestPersistentStore[string](t)
	putValues(t, inputEntries, testPersistentStore)

	entriesFromStore, err := testPersistentStore.Entries()
	if err != nil {
		t.Fatalf("Failed to get stored entries from the persistent storage: %v", err)
	}

	compareStoredEntriesWithInputEntries(t, inputEntries, entriesFromStore)
}

// Testing PersistentStore.Count()
func TestPersistentStoreCount(t *testing.T) {
	testPersistentStore := createTestPersistentStore[string](t)
	putValues(t, inputEntries, testPersistentStore)

	listFromStore, err := testPersistentStore.List()
	if err != nil {
		t.Fatalf("Failed to get the list of stored values from the persistent storage: %v", err)
	}

	numOfEntries, err := testPersistentStore.Count()
	if err != nil {
		t.Fatalf("Failed to get number of stored values from the persistent storage: %v", err)
	}

	if numOfEntries != len(listFromStore) {
		t.Fatalf(
			"Number of entries in the storage: %d is different from the value returned from the `count` method: %d",
			numOfEntries,
			len(listFromStore),
		)
	}
}
