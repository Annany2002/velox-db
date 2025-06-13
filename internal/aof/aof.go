package aof

import (
	"bufio"
	"container/list"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/Annany2002/velox-db/internal/resp"
	"github.com/Annany2002/velox-db/internal/store"
)

// Aof handles the Append-Only File persistence
type Aof struct {
	file *os.File
	path string
}

// New creates a new Aof manager
func New(path string) (*Aof, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0777); err != nil {
		return nil, err
	}
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
	return a.file.Close()
}

// Write appends a command to the AOF file
func (a *Aof) Write(obj resp.Object) error {
	bytes := obj.ToBytes()
	_, err := a.file.Write(bytes)
	return err
}

// Rewrite creates a new, compact AOF file.
func (a *Aof) Rewrite(s *store.Store) error {
	tmpPath := a.path + ".tmp"
	tmpFile, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}
	defer tmpFile.Close()

	s.ForEach(func(key string, value interface{}) {
		var cmd resp.Object
		switch v := value.(type) {
		case []byte:
			cmd = resp.Object{
				Type: resp.ArrayPrefix,
				Array: []resp.Object{
					{Type: resp.BulkStringPrefix, Bulk: []byte("SET")},
					{Type: resp.BulkStringPrefix, Bulk: []byte(key)},
					{Type: resp.BulkStringPrefix, Bulk: v},
				},
			}
		
		case *list.List:
			elements := make([]resp.Object, 0, v.Len()+2)
			elements = append(elements, resp.Object{Type: resp.BulkStringPrefix, Bulk: []byte("RPUSH")})
			elements = append(elements, resp.Object{Type: resp.BulkStringPrefix, Bulk: []byte(key)})
			for e := v.Front(); e != nil; e = e.Next() {
				elements = append(elements, resp.Object{Type: resp.BulkStringPrefix, Bulk: e.Value.([]byte)})
			}
			cmd = resp.Object{Type: resp.ArrayPrefix, Array: elements}
		
		case map[string][]byte:
			// Generate a single HSET command with all field-value pairs for hashes.
			elements := make([]resp.Object, 0, 2*len(v)+2)
			elements = append(elements, resp.Object{Type: resp.BulkStringPrefix, Bulk: []byte("HSET")})
			elements = append(elements, resp.Object{Type: resp.BulkStringPrefix, Bulk: []byte(key)})
			for field, val := range v {
				elements = append(elements, resp.Object{Type: resp.BulkStringPrefix, Bulk: []byte(field)})
				elements = append(elements, resp.Object{Type: resp.BulkStringPrefix, Bulk: val})
			}
			cmd = resp.Object{Type: resp.ArrayPrefix, Array: elements}
		}

		if _, err := tmpFile.Write(cmd.ToBytes()); err != nil {
			fmt.Printf("Error writing to temp AOF file: %v\n", err)
		}
	})

	a.file.Close()
	if err := os.Rename(tmpPath, a.path); err != nil {
		return err
	}
	f, err := os.OpenFile(a.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return err
	}
	a.file = f
	return nil
}

// Load reads commands from the AOF file and returns them.
func Load(path string) (chan resp.Object, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("AOF file does not exist, creating new one...")
			os.Create(path)
			return nil, nil
		}
		return nil, err
	}

	commands := make(chan resp.Object)
	go func() {
		defer file.Close()
		defer close(commands)
		reader := resp.NewReader(bufio.NewReader(file))
		for {
			obj, err := reader.ReadObject()
			if err != nil {
				if err == io.EOF {
					break
				}
				return
			}
			commands <- obj
		}
	}()
	return commands, nil
}