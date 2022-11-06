package badgers3

import (
	"fmt"
	"github.com/dgraph-io/badger"
	"time"
)

var (
	db = getCacheDb()
)

// handleError will attempt to handle and show any errors thrown by BadgerDB
func handleCacheError(err error) {
	if err != nil {
		return
	}
}

// getCacheDb will open a new BadgerDB for the current S3 instance
func getCacheDb() *badger.DB {
	db, err := badger.Open(badger.DefaultOptions("/tmp/badger-s3"))
	if err != nil {
		_ = fmt.Errorf("unable to open badgerdb, check that there isn't already an instance running")
	}

	return db
}

// setCacheEntry will set an object into the Badger DB
func setCacheEntry(key []byte, data []byte, ttl time.Duration) {
	err := db.Update(func(txn *badger.Txn) error {
		e := badger.NewEntry(key, data).WithTTL(ttl).WithDiscard()
		err := txn.SetEntry(e)
		handleCacheError(err)

		return err
	})

	handleCacheError(err)
}

// getCacheEntry will return a cache entry as a string
func getCacheEntry(key []byte) (model *string) {
	var valCopy []byte
	err := db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		handleCacheError(err)

		if err == nil {
			err = item.Value(func(val []byte) error {
				valCopy = append([]byte{}, val...)
				return nil
			})
		}

		return err
	})

	handleCacheError(err)
	if err == nil {
		strVal := string(valCopy)
		return &strVal
	}

	return nil
}

// isCacheEntryExistent will return true when the given key exists in the cache storage, false otherwise
func isCacheEntryExistent(key []byte) bool {
	err := db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(key)
		return err
	})

	// If we have no error using txn.Get for a key then the key exists
	// Otherwise the key does not exist
	return err == nil
}
