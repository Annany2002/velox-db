package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
)

// Define RESP data type prefixes
const (
	SimpleStringPrefix = '+'
	ErrorPrefix        = '-'
	IntegerPrefix      = ':'
	BulkStringPrefix   = '$'
	ArrayPrefix        = '*'
)

// RESPObject represents any RESP data type
type RESPObject struct {
	Type  byte
	Str   []byte
	Num   int
	Bulk  []byte
	Array []RESPObject
}

// RESPReader wraps bufio.Reader to provide RESP parsing
type RESPReader struct {
	reader *bufio.Reader
}

// NewRESPReader creates a new RESPReader
func NewRESPReader(r io.Reader) *RESPReader {
	return &RESPReader{reader: bufio.NewReader(r)}
}

// readLine reads a line ending in \r\n
func (r *RESPReader) readLine() ([]byte, error) {
	line, err := r.reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	// Return the line without the trailing \r\n
	return line[:len(line)-2], nil
}

// readInteger reads an integer value after the prefix
func (r *RESPReader) readInteger() (int, error) {
	line, err := r.readLine()
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(string(line))
}

// ReadObject is the main entry point for parsing a RESP object
func (r *RESPReader) ReadObject() (RESPObject, error) {
	prefix, err := r.reader.ReadByte()
	if err != nil {
		return RESPObject{}, err
	}

	switch prefix {
	case SimpleStringPrefix:
		str, err := r.readLine()
		return RESPObject{Type: prefix, Str: str}, err
	case ErrorPrefix:
		str, err := r.readLine()
		return RESPObject{Type: prefix, Str: str}, err
	case IntegerPrefix:
		num, err := r.readInteger()
		return RESPObject{Type: prefix, Num: num}, err
	case BulkStringPrefix:
		length, err := r.readInteger()
		if err != nil {
			return RESPObject{}, err
		}
		// Handle null bulk strings
		if length == -1 {
			return RESPObject{Type: prefix, Bulk: nil}, nil
		}
		// Read the bulk string data plus the trailing \r\n
		buf := make([]byte, length+2)
		_, err = io.ReadFull(r.reader, buf)
		if err != nil {
			return RESPObject{}, err
		}
		return RESPObject{Type: prefix, Bulk: buf[:length]}, nil
	case ArrayPrefix:
		count, err := r.readInteger()
		if err != nil {
			return RESPObject{}, err
		}
		// Handle null or empty arrays
		if count <= 0 {
			return RESPObject{Type: prefix, Array: []RESPObject{}}, nil
		}
		arr := make([]RESPObject, count)
		for i := 0; i < count; i++ {
			arr[i], err = r.ReadObject()
			if err != nil {
				return RESPObject{}, err
			}
		}
		return RESPObject{Type: prefix, Array: arr}, nil
	default:
		return RESPObject{}, fmt.Errorf("invalid or unsupported RESP prefix: %q", prefix)
	}
}