package main

import (
	"os"
	"strconv"
	"sync"
)

// Aof handles the Append-Only File persistence
type Aof struct {
	file *os.File
	mu   sync.Mutex
}

// NewAof creates a new Aof manager
func NewAof(path string) (*Aof, error) {
	// Open the file with flags for writing, creating if it doesn't exist, and appending
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}
	return &Aof{
		file: f,
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

	// Serialize the RESPObject back to its byte representation
	bytes := obj.ToBytes()

	_, err := a.file.Write(bytes)
	return err
}

// ToBytes serializes an RESPObject back to the RESP wire format
func (o RESPObject) ToBytes() []byte {
	switch o.Type {
	case ArrayPrefix:
		var bytes []byte
		// Append array prefix and count
		bytes = append(bytes, ArrayPrefix)
		bytes = append(bytes, []byte(strconv.Itoa(len(o.Array)))...)
		bytes = append(bytes, '\r', '\n')
		// Append each element
		for _, elem := range o.Array {
			bytes = append(bytes, elem.ToBytes()...)
		}
		return bytes
	case BulkStringPrefix:
		var bytes []byte
		// Append bulk string prefix and length
		bytes = append(bytes, BulkStringPrefix)
		bytes = append(bytes, []byte(strconv.Itoa(len(o.Bulk)))...)
		bytes = append(bytes, '\r', '\n')
		// Append the data and final CRLF
		bytes = append(bytes, o.Bulk...)
		bytes = append(bytes, '\r', '\n')
		return bytes
	default:
		// For simplicity, we assume commands are arrays of bulk strings
		return []byte{}
	}
}