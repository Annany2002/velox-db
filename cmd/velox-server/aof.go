package main

import (
	"container/list"
	"os"
	"strconv"
	"sync"
)

// Aof handles the Append-Only File persistence
type Aof struct {
	file *os.File
	path string // Keep track of the file path
	mu   sync.Mutex
}

// NewAof creates a new Aof manager
func NewAof(path string) (*Aof, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}
	return &Aof{
		file: f,
		path: path,
	}, nil
}

// Close closes the AOF file
func (a *Aof) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.file.Close()
}

// Write appends a command to the AOF file
func (a *Aof) Write(obj RESPObject) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	bytes := obj.ToBytes()
	_, err := a.file.Write(bytes)
	return err
}

// Rewrite now handles different data types in the store.
func (a *Aof) Rewrite() error {
	mu.RLock()
	defer mu.RUnlock()

	tmpPath := a.path + ".tmp"
	tmpFile, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}
	defer tmpFile.Close()

	// Iterate over the data and generate the appropriate command for each type.
	for key, value := range data {
		var cmd RESPObject

		switch v := value.(type) {
		case []byte:
			// Generate a SET command for strings.
			cmd = RESPObject{
				Type: ArrayPrefix,
				Array: []RESPObject{
					{Type: BulkStringPrefix, Bulk: []byte("SET")},
					{Type: BulkStringPrefix, Bulk: []byte(key)},
					{Type: BulkStringPrefix, Bulk: v},
				},
			}
		case *list.List:
			// Generate a single RPUSH command with all elements for lists.
			elements := make([]RESPObject, 0, v.Len()+2)
			elements = append(elements, RESPObject{Type: BulkStringPrefix, Bulk: []byte("RPUSH")})
			elements = append(elements, RESPObject{Type: BulkStringPrefix, Bulk: []byte(key)})
			for e := v.Front(); e != nil; e = e.Next() {
				elements = append(elements, RESPObject{Type: BulkStringPrefix, Bulk: e.Value.([]byte)})
			}
			cmd = RESPObject{Type: ArrayPrefix, Array: elements}
		}

		if _, err := tmpFile.Write(cmd.ToBytes()); err != nil {
			return err
		}
	}

	// Atomically replace the old file.
	a.mu.Lock()
	a.file.Close()
	a.mu.Unlock()

	if err := os.Rename(tmpPath, a.path); err != nil {
		return err
	}

	f, err := os.OpenFile(a.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.file = f
	a.mu.Unlock()

	return nil
}

// ToBytes serializes an RESPObject back to the RESP wire format
func (o RESPObject) ToBytes() []byte {
	switch o.Type {
	case ArrayPrefix:
		var bytes []byte
		bytes = append(bytes, ArrayPrefix)
		bytes = append(bytes, []byte(strconv.Itoa(len(o.Array)))...)
		bytes = append(bytes, '\r', '\n')
		for _, elem := range o.Array {
			bytes = append(bytes, elem.ToBytes()...)
		}
		return bytes
	case BulkStringPrefix:
		var bytes []byte
		bytes = append(bytes, BulkStringPrefix)
		bytes = append(bytes, []byte(strconv.Itoa(len(o.Bulk)))...)
		bytes = append(bytes, '\r', '\n')
		bytes = append(bytes, o.Bulk...)
		bytes = append(bytes, '\r', '\n')
		return bytes
	default:
		return []byte{}
	}
}