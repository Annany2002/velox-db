package server

import (
	"container/list"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"

	"github.com/Annany2002/velox-db/internal/aof"
	"github.com/Annany2002/velox-db/internal/resp"
	"github.com/Annany2002/velox-db/internal/store"
)

// Server holds the dependencies for the database server
type Server struct {
	store *store.Store
	aof   *aof.Aof
}

// New creates a new Server
func New(s *store.Store, a *aof.Aof) *Server {
	return &Server{
		store: s,
		aof:   a,
	}
}

// Start begins listening for client connections
func (s *Server) Start(addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to bind to address %s: %w", addr, err)
	}
	defer listener.Close()
	log.Printf("VeloxDB server started on %s", addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %s", err.Error())
			continue
		}
		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	log.Printf("Client connected: %s", conn.RemoteAddr())

	reader := resp.NewReader(conn)

	for {
		obj, err := reader.ReadObject()
		if err != nil {
			if err != io.EOF {
				log.Printf("Error reading command from client: %s", err.Error())
			}
			return
		}

		response, err := s.applyCommand(obj)
		if err != nil {
			conn.Write([]byte(fmt.Sprintf("-%s\r\n", err.Error())))
			continue
		}

		conn.Write(response)

		command := strings.ToUpper(string(obj.Array[0].Bulk))
		writeCmds := map[string]bool{"SET": true, "DEL": true, "LPUSH": true, "RPUSH": true, "LPOP": true, "RPOP": true}
		if writeCmds[command] {
			if err := s.aof.Write(obj); err != nil {
				log.Printf("Failed to write to AOF: %s", err.Error())
			}
		}
	}
}

func (s *Server) applyCommand(obj resp.Object) ([]byte, error) {
	if obj.Type != resp.ArrayPrefix || len(obj.Array) == 0 {
		return nil, fmt.Errorf("invalid command format: not an array")
	}
	commandObj := obj.Array[0]
	if commandObj.Type != resp.BulkStringPrefix {
		return nil, fmt.Errorf("invalid command format: command is not a bulk string")
	}
	command := strings.ToUpper(string(commandObj.Bulk))
	args := obj.Array[1:]

	switch command {
	case "PING":
		return []byte("+PONG\r\n"), nil
	case "SET":
		if len(args) != 2 {
			return nil, fmt.Errorf("ERR wrong number of arguments for 'set' command")
		}
		key, value := string(args[0].Bulk), args[1].Bulk
		s.store.Set(key, []byte(value))
		return []byte("+OK\r\n"), nil
	case "GET":
		if len(args) != 1 {
			return nil, fmt.Errorf("ERR wrong number of arguments for 'get' command")
		}
		key := string(args[0].Bulk)
		value, ok := s.store.Get(key)
		if !ok {
			return []byte("$-1\r\n"), nil // Handles both key not found and wrong type
		}
		return []byte(fmt.Sprintf("$%d\r\n%s\r\n", len(value), value)), nil
	case "DEL":
		if len(args) < 1 {
			return nil, fmt.Errorf("ERR wrong number of arguments for 'del' command")
		}
		var deletedCount int
		for _, arg := range args {
			if s.store.Del(string(arg.Bulk)) {
				deletedCount++
			}
		}
		return []byte(fmt.Sprintf(":%d\r\n", deletedCount)), nil
	case "LPUSH", "RPUSH":
		if len(args) < 2 {
			return nil, fmt.Errorf("ERR wrong number of arguments for '%s' command", strings.ToLower(command))
		}
		key := string(args[0].Bulk)
		values := args[1:]
		s.store.Lock()
		defer s.store.Unlock()
		listValue, err := s.store.GetOrCreateList(key)
		if err != nil {
			return nil, fmt.Errorf(resp.WRONGTYPE_ERROR)
		}
		for _, v := range values {
			if command == "LPUSH" {
				listValue.PushFront(v.Bulk)
			} else {
				listValue.PushBack(v.Bulk)
			}
		}
		return []byte(fmt.Sprintf(":%d\r\n", listValue.Len())), nil
	case "LPOP", "RPOP":
		if len(args) != 1 {
			return nil, fmt.Errorf("ERR wrong number of arguments for '%s' command", strings.ToLower(command))
		}
		key := string(args[0].Bulk)
		s.store.Lock()
		defer s.store.Unlock()
		listValue, ok := s.store.GetList(key)
		if !ok || listValue.Len() == 0 {
			return []byte("$-1\r\n"), nil
		}
		var element *list.Element
		if command == "LPOP" {
			element = listValue.Front()
		} else {
			element = listValue.Back()
		}
		listValue.Remove(element)
		if listValue.Len() == 0 {
			s.store.Del(key)
		}
		value := element.Value.([]byte)
		return []byte(fmt.Sprintf("$%d\r\n%s\r\n", len(value), value)), nil
	case "LLEN":
		if len(args) != 1 {
			return nil, fmt.Errorf("ERR wrong number of arguments for 'llen' command")
		}
		key := string(args[0].Bulk)
		listValue, ok := s.store.GetList(key)
		if !ok {
			return []byte(":0\r\n"), nil
		}
		return []byte(fmt.Sprintf(":%d\r\n", listValue.Len())), nil
	case "LINDEX":
		// Implementation for LINDEX
		if len(args) != 2 {return nil, fmt.Errorf("ERR wrong number of arguments")}
		key := string(args[0].Bulk)
		index, err := strconv.Atoi(string(args[1].Bulk))
		if err != nil { return nil, fmt.Errorf("ERR value is not an integer")}
		listValue, ok := s.store.GetList(key)
		if !ok { return []byte("$-1\r\n"), nil}
		if index < 0 { index = listValue.Len() + index }
		if index < 0 || index >= listValue.Len() { return []byte("$-1\r\n"), nil}
		i := 0
		for e := listValue.Front(); e != nil; e = e.Next() {
			if i == index {
				value := e.Value.([]byte)
				return []byte(fmt.Sprintf("$%d\r\n%s\r\n", len(value), value)), nil
			}
			i++
		}
		return []byte("$-1\r\n"), nil
	case "LRANGE":
		// Implementation for LRANGE
		if len(args) != 3 {return nil, fmt.Errorf("ERR wrong number of arguments")}
		key := string(args[0].Bulk)
		start, err1 := strconv.Atoi(string(args[1].Bulk))
		stop, err2 := strconv.Atoi(string(args[2].Bulk))
		if err1 != nil || err2 != nil {return nil, fmt.Errorf("ERR value is not an integer")}
		listValue, ok := s.store.GetList(key)
		if !ok {return []byte("*0\r\n"), nil}
		listLen := listValue.Len()
		if start < 0 {start = listLen + start}
		if stop < 0 {stop = listLen + stop}
		if start < 0 {start = 0}
		if stop >= listLen {stop = listLen-1}
		var results []resp.Object
		if start > stop || start >= listLen {return []byte("*0\r\n"), nil}
		i := 0
		for e := listValue.Front(); e != nil && i <= stop; e = e.Next() {
			if i >= start {
				results = append(results, resp.Object{Type: resp.BulkStringPrefix, Bulk: e.Value.([]byte)})
			}
			i++
		}
		return resp.Object{Type: resp.ArrayPrefix, Array: results}.ToBytes(), nil

	case "REWRITEAOF":
		if err := s.aof.Rewrite(s.store); err != nil {
			return nil, fmt.Errorf("ERR failed to rewrite AOF: %v", err)
		}
		return []byte("+OK\r\n"), nil
	default:
		return nil, fmt.Errorf("ERR unknown command '%s'", command)
	}
}

// ApplyCommandForLoad is a special version of applyCommand used only during AOF loading.
// It doesn't write to the network or the AOF file.
func (s *Server) ApplyCommandForLoad(obj resp.Object) error {
	_, err := s.applyCommand(obj)
	return err
}