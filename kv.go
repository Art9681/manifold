package main

import (
	"log"

	badger "github.com/dgraph-io/badger/v4"
)

var db *badger.DB

func OpenKvStore() {
	// Open the Badger database located in the /tmp/badger directory.
	// It will be created if it doesn't exist.
	opts := badger.Options{
		Dir:            "/tmp/badger",
		IndexCacheSize: 10 << 20, // 10 MB
	}

	db, err := badger.Open(opts)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

}

func MemDB() {
	opts := badger.Options{
		InMemory:       true,
		IndexCacheSize: 10 << 20, // 10 MB
	}
	// Open a new in-memory database.
	db, err := badger.Open(opts)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
}
