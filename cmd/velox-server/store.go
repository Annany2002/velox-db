package main

import "sync"

var (
	// The data store for our key-value pairs.
	// We use `[]byte` for values because it's the natural format for network I/O.
	data = make(map[string]interface{})

	// A RWMutex (Read-Write Mutex) to protect the 'data' map.
	// It allows multiple concurrent readers but only one writer.
	mu = &sync.RWMutex{}
)