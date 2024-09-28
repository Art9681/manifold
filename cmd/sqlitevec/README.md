# go-sqlite full text search and vector embeddings example

To enable FTS5 with the github.com/mattn/go-sqlite3 package in Go, you need to build the SQLite3 library with the FTS5 extension enabled. 

Run the example with the following command:
```
$ go run -tags "sqlite_fts5" main.go
```

Build the example using:
```
$ go build -tags "sqlite_fts5"
```