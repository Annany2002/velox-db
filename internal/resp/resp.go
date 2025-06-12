package resp

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

// Object represents any RESP data type
type Object struct {
	Type  byte
	Str   []byte
	Num   int
	Bulk  []byte
	Array []Object
}

// Reader wraps bufio.Reader to provide RESP parsing
type Reader struct {
	reader *bufio.Reader
}

// NewReader creates a new Reader
func NewReader(r io.Reader) *Reader {
	return &Reader{reader: bufio.NewReader(r)}
}

func (r *Reader) readLine() ([]byte, error) {
	line, err := r.reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	return line[:len(line)-2], nil
}

func (r *Reader) readInteger() (int, error) {
	line, err := r.readLine()
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(string(line))
}

// ReadObject is the main entry point for parsing a RESP object
func (r *Reader) ReadObject() (Object, error) {
	prefix, err := r.reader.ReadByte()
	if err != nil {
		return Object{}, err
	}

	switch prefix {
	case SimpleStringPrefix:
		str, err := r.readLine()
		return Object{Type: prefix, Str: str}, err
	case ErrorPrefix:
		str, err := r.readLine()
		return Object{Type: prefix, Str: str}, err
	case IntegerPrefix:
		num, err := r.readInteger()
		return Object{Type: prefix, Num: num}, err
	case BulkStringPrefix:
		length, err := r.readInteger()
		if err != nil {
			return Object{}, err
		}
		if length == -1 {
			return Object{Type: prefix, Bulk: nil}, nil
		}
		buf := make([]byte, length+2)
		_, err = io.ReadFull(r.reader, buf)
		if err != nil {
			return Object{}, err
		}
		return Object{Type: prefix, Bulk: buf[:length]}, nil
	case ArrayPrefix:
		count, err := r.readInteger()
		if err != nil {
			return Object{}, err
		}
		if count <= 0 {
			return Object{Type: prefix, Array: []Object{}}, nil
		}
		arr := make([]Object, count)
		for i := 0; i < count; i++ {
			arr[i], err = r.ReadObject()
			if err != nil {
				return Object{}, err
			}
		}
		return Object{Type: prefix, Array: arr}, nil
	default:
		return Object{}, fmt.Errorf("invalid or unsupported RESP prefix: %q", prefix)
	}
}

// ToBytes serializes an Object back to the RESP wire format
func (o Object) ToBytes() []byte {
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

// WRONGTYPE_ERROR is a standard Redis error message
const WRONGTYPE_ERROR = "WRONGTYPE Operation against a key holding the wrong kind of value"