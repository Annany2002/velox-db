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

// commandFunc defines the signature for a function that handles a command.
type commandFunc func(args []resp.Object) ([]byte, error)

// Server holds the dependencies and state for the database server.
type Server struct {
	store    *store.Store
	aof      *aof.Aof
	commands map[string]commandFunc
}

// New creates and initializes a new Server.
func New(s *store.Store, a *aof.Aof) *Server {
	srv := &Server{
		store: s,
		aof:   a,
	}
	// The command table is populated here.
	srv.commands = map[string]commandFunc{
		"PING":       srv.handlePing,
		"SET":        srv.handleSet,
		"GET":        srv.handleGet,
		"DEL":        srv.handleDel,
		"LPUSH":      srv.handleLPush,
		"RPUSH":      srv.handleRPush,
		"LPOP":       srv.handleLPop,
		"RPOP":       srv.handleRPop,
		"LLEN":       srv.handleLLen,
		"LINDEX":     srv.handleLIndex,
		"LRANGE":     srv.handleLRange,
		"REWRITEAOF": srv.handleRewriteAOF,
	}
	return srv
}

// Start begins listening for client connections.
func (s *Server) Start(addr string) error {
	// ... This function remains unchanged ...
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

// applyCommand is now a simple dispatcher using the command table.
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

	// Look up the command in the table.
	cmdFunc, ok := s.commands[command]
	if !ok {
		return nil, fmt.Errorf("ERR unknown command '%s'", command)
	}

	// Execute the command's handler function.
	return cmdFunc(args)
}

// ApplyCommandForLoad is used only during AOF loading.
func (s *Server) ApplyCommandForLoad(obj resp.Object) error {
	// This function uses the new applyCommand dispatcher.
	_, err := s.applyCommand(obj)
	return err
}

// --- Individual Command Handlers ---

func (s *Server) handlePing(args []resp.Object) ([]byte, error) {
	return []byte("+PONG\r\n"), nil
}

func (s *Server) handleSet(args []resp.Object) ([]byte, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("ERR wrong number of arguments for 'set' command")
	}
	key, value := string(args[0].Bulk), args[1].Bulk
	s.store.Set(key, value)
	return []byte("+OK\r\n"), nil
}

func (s *Server) handleGet(args []resp.Object) ([]byte, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("ERR wrong number of arguments for 'get' command")
	}
	key := string(args[0].Bulk)
	value, ok := s.store.Get(key)
	if !ok {
		return []byte("$-1\r\n"), nil // Handles key not found or wrong type
	}
	return []byte(fmt.Sprintf("$%d\r\n%s\r\n", len(value), value)), nil
}

func (s *Server) handleDel(args []resp.Object) ([]byte, error) {
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
}

func (s *Server) handleLPush(args []resp.Object) ([]byte, error) {
	return s.pushToList("LPUSH", args)
}

func (s *Server) handleRPush(args []resp.Object) ([]byte, error) {
	return s.pushToList("RPUSH", args)
}

func (s *Server) pushToList(command string, args []resp.Object) ([]byte, error) {
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
}

func (s *Server) handleLPop(args []resp.Object) ([]byte, error) {
	return s.popFromList("LPOP", args)
}

func (s *Server) handleRPop(args []resp.Object) ([]byte, error) {
	return s.popFromList("RPOP", args)
}

func (s *Server) popFromList(command string, args []resp.Object) ([]byte, error) {
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
}

func (s *Server) handleLLen(args []resp.Object) ([]byte, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("ERR wrong number of arguments for 'llen' command")
	}
	key := string(args[0].Bulk)
	listValue, ok := s.store.GetList(key)
	if !ok {
		return []byte(":0\r\n"), nil
	}
	return []byte(fmt.Sprintf(":%d\r\n", listValue.Len())), nil
}

func (s *Server) handleLIndex(args []resp.Object) ([]byte, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("ERR wrong number of arguments for 'lindex' command")
	}
	key := string(args[0].Bulk)
	index, err := strconv.Atoi(string(args[1].Bulk))
	if err != nil {
		return nil, fmt.Errorf("ERR value is not an integer or out of range")
	}
	listValue, ok := s.store.GetList(key)
	if !ok {
		return []byte("$-1\r\n"), nil
	}
	if index < 0 {
		index = listValue.Len() + index
	}
	if index < 0 || index >= listValue.Len() {
		return []byte("$-1\r\n"), nil
	}
	i := 0
	for e := listValue.Front(); e != nil; e = e.Next() {
		if i == index {
			value := e.Value.([]byte)
			return []byte(fmt.Sprintf("$%d\r\n%s\r\n", len(value), value)), nil
		}
		i++
	}
	return []byte("$-1\r\n"), nil
}

func (s *Server) handleLRange(args []resp.Object) ([]byte, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("ERR wrong number of arguments for 'lrange' command")
	}
	key := string(args[0].Bulk)
	start, err1 := strconv.Atoi(string(args[1].Bulk))
	stop, err2 := strconv.Atoi(string(args[2].Bulk))
	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("ERR value is not an integer or out of range")
	}
	listValue, ok := s.store.GetList(key)
	if !ok {
		return []byte("*0\r\n"), nil
	}
	listLen := listValue.Len()
	if start < 0 {
		start = listLen + start
	}
	if stop < 0 {
		stop = listLen + stop
	}
	if start < 0 {
		start = 0
	}
	if stop >= listLen {
		stop = listLen - 1
	}
	var results []resp.Object
	if start > stop || start >= listLen {
		return []byte("*0\r\n"), nil
	}
	i := 0
	for e := listValue.Front(); e != nil && i <= stop; e = e.Next() {
		if i >= start {
			results = append(results, resp.Object{Type: resp.BulkStringPrefix, Bulk: e.Value.([]byte)})
		}
		i++
	}
	return resp.Object{Type: resp.ArrayPrefix, Array: results}.ToBytes(), nil
}

func (s *Server) handleRewriteAOF(args []resp.Object) ([]byte, error) {
	if err := s.aof.Rewrite(s.store); err != nil {
		return nil, fmt.Errorf("ERR failed to rewrite AOF: %v", err)
	}
	return []byte("+OK\r\n"), nil
}