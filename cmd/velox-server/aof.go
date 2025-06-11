package main

import (
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

// Rewrite creates a new, compact AOF file.
func (a *Aof) Rewrite() error {
	// We need a global read lock on the main data store while rewriting.
	mu.RLock()
	defer mu.RUnlock()

	// 1. Create a temporary file.
	tmpPath := a.path + ".tmp"
	tmpFile, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}
	defer tmpFile.Close()

	// 2. Iterate over the in-memory data and write a SET command for each key.
	for key, value := range data {
		// Construct a SET command as a RESPObject
		setCmd := RESPObject{
			Type: ArrayPrefix,
			Array: []RESPObject{
				{Type: BulkStringPrefix, Bulk: []byte("SET")},
				{Type: BulkStringPrefix, Bulk: []byte(key)},
				{Type: BulkStringPrefix, Bulk: value},
			},
		}
		
		// Write the command to the temporary file
		if _, err := tmpFile.Write(setCmd.ToBytes()); err != nil {
			return err
		}
	}
	
	// 3. Atomically replace the old AOF file with the new one.
	// First, we must close the current AOF file before renaming.
	a.mu.Lock()
	a.file.Close()
	a.mu.Unlock()

	if err := os.Rename(tmpPath, a.path); err != nil {
		return err
	}

	// 4. Re-open the AOF file for appending future commands.
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
// (This function remains unchanged)
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