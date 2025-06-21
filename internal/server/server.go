package server

import (
	"container/list"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

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
		"PING": srv.handlePing,

		// String commands
		"SET": srv.handleSet,
		"GET": srv.handleGet,
		"DEL": srv.handleDel,

		// List commands
		"LPUSH":  srv.handleLPush,
		"RPUSH":  srv.handleRPush,
		"LPOP":   srv.handleLPop,
		"RPOP":   srv.handleRPop,
		"LLEN":   srv.handleLLen,
		"LINDEX": srv.handleLIndex,
		"LRANGE": srv.handleLRange,

		// AOF commands
		"REWRITEAOF": srv.handleRewriteAOF,

		// Hash commands
		"HSET":    srv.handleHSet,
		"HGET":    srv.handleHGet,
		"HGETALL": srv.handleHGetAll,
		"HDEL":    srv.handleHDel,

		// Set commands
		"SADD":      srv.handleSAdd,
		"SREM":      srv.handleSRem,
		"SISMEMBER": srv.handleSIsMember,
		"SMEMBERS":  srv.handleSMembers,

		// ZSet commands
		"ZADD":          srv.handleZAdd,
		"ZREM":          srv.handleZRem,
		"ZCARD":         srv.handleZCard,
		"ZRANGE":        srv.handleZRange,
		"ZSCORE":        srv.handleZScore,
		"ZREVRANGE":     srv.handleZRevRange,
		"ZCOUNT":        srv.handleZCount,
		"ZRANGEBYSCORE": srv.handleZRangeByScore,

		// Expiration commands.
		"EXPIRE": srv.handleExpire,
		"TTL":    srv.handleTTL,
		// PEXPIREAT is used for AOF loading but not typically user-facing.
		// We add it to the command table to allow AOF recovery to work.
		"PEXPIREAT": srv.handlePExpireAt,
	}
	return srv
}

// Start begins listening for client connections.
func (s *Server) Start(addr string) error {
	// Launch the active expiration janitor.
	go s.startJanitor()

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

// handleConnection handles a client connection.
// It reads commands from the client and writes responses.
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
		writeCmds := map[string]bool{
			"SET": true, "DEL": true,
			"LPUSH": true, "RPUSH": true, "LPOP": true, "RPOP": true,
			"HSET": true, "HDEL": true,
			"SADD": true, "SREM": true,
			"EXPIRE": true, "PEXPIREAT": true,
			"ZADD": true, "ZREM": true, "ZCARD": true, "ZSCORE": true, "ZRANGE": true,
		}
		if writeCmds[command] {
			if err := s.aof.Write(obj); err != nil {
				log.Printf("Failed to write to AOF: %s", err.Error())
			}
		}
	}
}

// startJanitor begins a background process to actively expire keys.
func (s *Server) startJanitor() {
	// In a real system, the interval would be configurable.
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Redis samples a small number of keys. We'll do the same.
		keys := s.store.RandomKeysWithExpiry(20)
		if len(keys) == 0 {
			continue
		}

		var expiredCount int
		s.store.Lock()
		for _, key := range keys {
			// The isExpired check handles deletion internally.
			if s.store.IsExpired(key) {
				expiredCount++
			}
		}
		s.store.Unlock()

		if expiredCount > 0 {
			log.Printf("Janitor cleaned up %d expired keys", expiredCount)
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

// handleExpire handles the EXPIRE command.
// EXPIRE key seconds
func (s *Server) handleExpire(args []resp.Object) ([]byte, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("ERR wrong number of arguments for 'expire' command")
	}
	key := string(args[0].Bulk)
	seconds, err := strconv.Atoi(string(args[1].Bulk))
	if err != nil {
		return nil, fmt.Errorf("ERR value is not an integer or out of range")
	}

	// Check if key exists first using Get (covers string type)
	_, keyExists := s.store.Get(key)
	if !keyExists {
		// Try other types (list, hash, set)
		if _, ok := s.store.GetList(key); !ok {
			if _, ok := s.store.GetHash(key); !ok {
				if _, ok := s.store.GetSet(key); !ok {
					return []byte(":0\r\n"), nil // No key to expire
				}
			}
		}
	}

	expiry := time.Now().Add(time.Duration(seconds) * time.Second)
	s.store.SetExpiry(key, expiry)

	return []byte(":1\r\n"), nil // Success
}

// handleTTL handles the TTL command.
// TTL key
func (s *Server) handleTTL(args []resp.Object) ([]byte, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("ERR wrong number of arguments for 'ttl' command")
	}
	key := string(args[0].Bulk)

	s.store.Lock()
	isExpired := s.store.IsExpired(key) // Check and delete if expired
	s.store.Unlock()

	if isExpired {
		return []byte(":-2\r\n"), nil // Key was expired and just deleted
	}

	expiry, ok := s.store.GetExpiry(key)
	if !ok {
		// Before returning -1, we must confirm the key exists.
		keyExists := false
		if _, ok := s.store.Get(key); ok {
			keyExists = true
		} else if _, ok := s.store.GetList(key); ok {
			keyExists = true
		} else if _, ok := s.store.GetHash(key); ok {
			keyExists = true
		} else if _, ok := s.store.GetSet(key); ok {
			keyExists = true
		}
		if !keyExists {
			return []byte(":-2\r\n"), nil // Key does not exist
		}
		return []byte(":-1\r\n"), nil // Key exists but has no expiry
	}

	ttl := time.Until(expiry).Seconds()
	return []byte(fmt.Sprintf(":%d\r\n", int(ttl))), nil
}

// handlePExpireAt handles the PEXPIREAT command, used for AOF loading.
func (s *Server) handlePExpireAt(args []resp.Object) ([]byte, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("ERR wrong number of arguments for 'pexpireat' command")
	}
	key := string(args[0].Bulk)
	ms, err := strconv.ParseInt(string(args[1].Bulk), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("ERR value is not an integer or out of range")
	}

	expiry := time.UnixMilli(ms)
	s.store.SetExpiry(key, expiry)

	return []byte(":1\r\n"), nil
}

// --- Individual Command Handlers ---

// handlePing handles the PING command.
// PING
func (s *Server) handlePing(args []resp.Object) ([]byte, error) {
	return []byte("+PONG\r\n"), nil
}

// handleSet handles the SET command.
// SET key value
func (s *Server) handleSet(args []resp.Object) ([]byte, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("ERR wrong number of arguments for 'set' command")
	}
	key, value := string(args[0].Bulk), args[1].Bulk
	s.store.Set(key, value)
	return []byte("+OK\r\n"), nil
}

// handleGet handles the GET command.
// GET key
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

// handleDel handles the DEL command.
// DEL key [key ...]
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

// handleLPush handles the LPUSH command.
// LPUSH key value [value ...]
func (s *Server) handleLPush(args []resp.Object) ([]byte, error) {
	return s.pushToList("LPUSH", args)
}

// handleRPush handles the RPUSH command.
// RPUSH key value [value ...]
func (s *Server) handleRPush(args []resp.Object) ([]byte, error) {
	return s.pushToList("RPUSH", args)
}

// pushToList handles the LPUSH and RPUSH commands.
// LPUSH key value [value ...]
// RPUSH key value [value ...]
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

// handleLPop handles the LPOP command.
// LPOP key
func (s *Server) handleLPop(args []resp.Object) ([]byte, error) {
	return s.popFromList("LPOP", args)
}

// handleRPop handles the RPOP command.
// RPOP key
func (s *Server) handleRPop(args []resp.Object) ([]byte, error) {
	return s.popFromList("RPOP", args)
}

// popFromList handles the LPOP and RPOP commands.
// LPOP key
// RPOP key
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

// handleLLen handles the LLEN command.
// LLEN key
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

// handleLIndex handles the LINDEX command.
// LINDEX key index
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

// handleLRange handles the LRANGE command.
// LRANGE key start stop
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

// handleRewriteAOF handles the REWRITEAOF command.
// REWRITEAOF
func (s *Server) handleRewriteAOF(args []resp.Object) ([]byte, error) {
	if err := s.aof.Rewrite(s.store); err != nil {
		return nil, fmt.Errorf("ERR failed to rewrite AOF: %v", err)
	}
	return []byte("+OK\r\n"), nil
}

// handleHSet handles the HSET command.
// HSET key field value [field value ...]
func (s *Server) handleHSet(args []resp.Object) ([]byte, error) {
	if len(args) < 3 || len(args)%2 != 1 {
		return nil, fmt.Errorf("ERR wrong number of arguments for 'hset' command")
	}
	key := string(args[0].Bulk)

	s.store.Lock()
	defer s.store.Unlock()

	hash, err := s.store.GetOrCreateHash(key)
	if err != nil {
		return nil, fmt.Errorf(resp.WRONGTYPE_ERROR)
	}

	var fieldsAdded int
	for i := 1; i < len(args); i += 2 {
		field := string(args[i].Bulk)
		value := args[i+1].Bulk
		if _, ok := hash[field]; !ok {
			fieldsAdded++
		}
		hash[field] = value
	}

	return []byte(fmt.Sprintf(":%d\r\n", fieldsAdded)), nil
}

// handleHGet handles the HGET command.
// HGET key field
func (s *Server) handleHGet(args []resp.Object) ([]byte, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("ERR wrong number of arguments for 'hget' command")
	}
	key := string(args[0].Bulk)
	field := string(args[1].Bulk)

	hash, ok := s.store.GetHash(key)
	if !ok {
		return []byte("$-1\r\n"), nil // Key doesn't exist or is wrong type
	}

	value, ok := hash[field]
	if !ok {
		return []byte("$-1\r\n"), nil // Field doesn't exist
	}

	return []byte(fmt.Sprintf("$%d\r\n%s\r\n", len(value), value)), nil
}

// handleHGetAll handles the HGETALL command.
// HGETALL key
func (s *Server) handleHGetAll(args []resp.Object) ([]byte, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("ERR wrong number of arguments for 'hgetall' command")
	}
	key := string(args[0].Bulk)

	hash, ok := s.store.GetHash(key)
	if !ok {
		return []byte("*0\r\n"), nil // Return empty array for non-existent or wrong type key
	}

	// Create a RESP array of field-value pairs.
	results := make([]resp.Object, 0, 2*len(hash))
	for field, value := range hash {
		results = append(results, resp.Object{Type: resp.BulkStringPrefix, Bulk: []byte(field)})
		results = append(results, resp.Object{Type: resp.BulkStringPrefix, Bulk: value})
	}

	return resp.Object{Type: resp.ArrayPrefix, Array: results}.ToBytes(), nil
}

// handleHDel handles the HDEL command.
// HDEL key field [field ...]
func (s *Server) handleHDel(args []resp.Object) ([]byte, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("ERR wrong number of arguments for 'hdel' command")
	}
	key := string(args[0].Bulk)
	fields := args[1:]

	s.store.Lock()
	defer s.store.Unlock()

	hash, ok := s.store.GetHash(key)
	if !ok {
		return []byte(":0\r\n"), nil
	}

	var deletedCount int
	for _, fieldArg := range fields {
		field := string(fieldArg.Bulk)
		if _, ok := hash[field]; ok {
			delete(hash, field)
			deletedCount++
		}
	}

	// If the hash is now empty, delete the key itself.
	if len(hash) == 0 {
		s.store.Del(key)
	}

	return []byte(fmt.Sprintf(":%d\r\n", deletedCount)), nil
}

// handleSAdd handles the SADD command.
// SADD key member [member ...]
func (s *Server) handleSAdd(args []resp.Object) ([]byte, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("ERR wrong number of arguments for 'sadd' command")
	}
	key := string(args[0].Bulk)
	members := args[1:]

	s.store.Lock()
	defer s.store.Unlock()

	set, err := s.store.GetOrCreateSet(key)
	if err != nil {
		return nil, fmt.Errorf(resp.WRONGTYPE_ERROR)
	}

	var addedCount int
	for _, member := range members {
		if _, ok := set[string(member.Bulk)]; !ok {
			set[string(member.Bulk)] = struct{}{}
			addedCount++
		}
	}
	return []byte(fmt.Sprintf(":%d\r\n", addedCount)), nil
}

// handleSRem handles the SREM command.
// SREM key member [member ...]
func (s *Server) handleSRem(args []resp.Object) ([]byte, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("ERR wrong number of arguments for 'srem' command")
	}
	key := string(args[0].Bulk)
	members := args[1:]

	s.store.Lock()
	defer s.store.Unlock()

	set, ok := s.store.GetSet(key)
	if !ok {
		return []byte(":0\r\n"), nil
	}

	var removedCount int
	for _, member := range members {
		if _, ok := set[string(member.Bulk)]; ok {
			delete(set, string(member.Bulk))
			removedCount++
		}
	}

	// If the set is now empty, delete the key itself.
	if len(set) == 0 {
		s.store.Del(key)
	}

	return []byte(fmt.Sprintf(":%d\r\n", removedCount)), nil
}

// handleSIsMember handles the SISMEMBER command.
// SISMEMBER key member
func (s *Server) handleSIsMember(args []resp.Object) ([]byte, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("ERR wrong number of arguments for 'sismember' command")
	}
	key := string(args[0].Bulk)
	member := string(args[1].Bulk)

	set, ok := s.store.GetSet(key)
	if !ok {
		return []byte(":0\r\n"), nil // Key doesn't exist, so not a member.
	}

	if _, ok := set[member]; ok {
		return []byte(":1\r\n"), nil // Is a member.
	}

	return []byte(":0\r\n"), nil // Is not a member.
}

// handleSMembers handles the SMEMBERS command.
// SMEMBERS key
func (s *Server) handleSMembers(args []resp.Object) ([]byte, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("ERR wrong number of arguments for 'smembers' command")
	}
	key := string(args[0].Bulk)

	set, ok := s.store.GetSet(key)
	if !ok {
		return []byte("*0\r\n"), nil // Return empty array.
	}

	results := make([]resp.Object, 0, len(set))
	for member := range set {
		results = append(results, resp.Object{Type: resp.BulkStringPrefix, Bulk: []byte(member)})
	}

	return resp.Object{Type: resp.ArrayPrefix, Array: results}.ToBytes(), nil
}

// ZADD key score member [score member ...]
func (s *Server) handleZAdd(args []resp.Object) ([]byte, error) {
	if len(args) < 3 || len(args)%2 != 1 {
		return nil, fmt.Errorf("ERR wrong number of arguments for 'zadd' command")
	}
	key := string(args[0].Bulk)
	
	s.store.Lock()
	defer s.store.Unlock()
	z, err := s.store.GetOrCreateZSet(key)
	if err != nil {
		return nil, fmt.Errorf(resp.WRONGTYPE_ERROR)
	}

	var addedCount int
	for i := 1; i < len(args); i += 2 {
		score, err := strconv.ParseFloat(string(args[i].Bulk), 64)
		if err != nil {
			return nil, fmt.Errorf("ERR value is not a valid float")
		}
		member := string(args[i+1].Bulk)
		addedCount += z.Add(score, member)
	}

	return []byte(fmt.Sprintf(":%d\r\n", addedCount)), nil
}

// ZCARD key
func (s *Server) handleZCard(args []resp.Object) ([]byte, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("ERR wrong number of arguments for 'zcard' command")
	}
	key := string(args[0].Bulk)
	z, ok := s.store.GetZSet(key)
	if !ok {
		return []byte(":0\r\n"), nil
	}
	return []byte(fmt.Sprintf(":%d\r\n", z.Length())), nil
}

// ZRANGE key start stop [WITHSCORES]
func (s *Server) handleZRange(args []resp.Object) ([]byte, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("ERR wrong number of arguments for 'zrange' command")
	}
	key := string(args[0].Bulk)
	start, err1 := strconv.ParseInt(string(args[1].Bulk), 10, 64)
	stop, err2 := strconv.ParseInt(string(args[2].Bulk), 10, 64)
	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("ERR value is not an integer or out of range")
	}
	
	z, ok := s.store.GetZSet(key)
	if !ok {
		return []byte("*0\r\n"), nil // Empty array for non-existent key
	}

	nodes := z.GetRange(start, stop)
	results := make([]resp.Object, len(nodes))
	for i, node := range nodes {
		results[i] = resp.Object{Type: resp.BulkStringPrefix, Bulk: []byte(node.Member)}
	}

	return resp.Object{Type: resp.ArrayPrefix, Array: results}.ToBytes(), nil
}

// ZREM key member [member ...]
func (s *Server) handleZRem(args []resp.Object) ([]byte, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("ERR wrong number of arguments for 'zrem' command")
	}
	key := string(args[0].Bulk)
	members := args[1:]
	
	z, ok := s.store.GetZSet(key)
	if !ok {
		return []byte(":0\r\n"), nil
	}
	
	var removedCount int
	for _, member := range members {
		if z.Remove(string(member.Bulk)) {
			removedCount++
		}
	}
	
	return []byte(fmt.Sprintf(":%d\r\n", removedCount)), nil
}

// ZSCORE key member
func (s *Server) handleZScore(args []resp.Object) ([]byte, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("ERR wrong number of arguments for 'zscore' command")
	}
	key := string(args[0].Bulk)
	member := string(args[1].Bulk)

	z, ok := s.store.GetZSet(key)
	if !ok {
		return []byte("$-1\r\n"), nil // Null bulk string for non-existent key
	}

	score, ok := z.GetScore(member)
	if !ok {
		return []byte("$-1\r\n"), nil // Null bulk string for non-existent member
	}

	scoreStr := strconv.FormatFloat(score, 'f', -1, 64)
	return []byte(fmt.Sprintf("$%d\r\n%s\r\n", len(scoreStr), scoreStr)), nil
}

// ZREVRANGE key start stop [WITHSCORES]
func (s *Server) handleZRevRange(args []resp.Object) ([]byte, error) {
	// This logic is very similar to ZRANGE, but calls GetRangeRev.
	if len(args) < 3 {
		return nil, fmt.Errorf("ERR wrong number of arguments for 'zrevrange' command")
	}
	// ... parsing of start/stop and error handling ...
	key := string(args[0].Bulk)
	start, _ := strconv.ParseInt(string(args[1].Bulk), 10, 64)
	stop, _ := strconv.ParseInt(string(args[2].Bulk), 10, 64)

	z, ok := s.store.GetZSet(key)
	if !ok {
		return []byte("*0\r\n"), nil
	}

	nodes := z.GetRangeRev(start, stop)
	results := make([]resp.Object, len(nodes))
	for i, node := range nodes {
		results[i] = resp.Object{Type: resp.BulkStringPrefix, Bulk: []byte(node.Member)}
	}
	return resp.Object{Type: resp.ArrayPrefix, Array: results}.ToBytes(), nil
}

// ZCOUNT key min max
func (s *Server) handleZCount(args []resp.Object) ([]byte, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("ERR wrong number of arguments for 'zcount' command")
	}
	key := string(args[0].Bulk)
	min, err1 := strconv.ParseFloat(string(args[1].Bulk), 64)
	max, err2 := strconv.ParseFloat(string(args[2].Bulk), 64)
	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("ERR min or max is not a float")
	}

	z, ok := s.store.GetZSet(key)
	if !ok {
		return []byte(":0\r\n"), nil
	}

	count := z.CountInRange(min, max)
	return []byte(fmt.Sprintf(":%d\r\n", count)), nil
}

// ZRANGEBYSCORE key min max [LIMIT offset count]
func (s *Server) handleZRangeByScore(args []resp.Object) ([]byte, error) {
	// This command can get very complex with ( and +inf/-inf.
	// We will implement the basic inclusive version.
	if len(args) != 3 {
		return nil, fmt.Errorf("ERR wrong number of arguments")
	}
	key := string(args[0].Bulk)
	min, err1 := strconv.ParseFloat(string(args[1].Bulk), 64)
	max, err2 := strconv.ParseFloat(string(args[2].Bulk), 64)
	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("ERR min or max is not a float")
	}

	z, ok := s.store.GetZSet(key)
	if !ok {
		return []byte("*0\r\n"), nil
	}

	// This is inefficient without a dedicated skip list method, but works for now.
	var results []resp.Object
	nodes := z.GetRange(0, -1) // Get all nodes
	for _, node := range nodes {
		if node.Score >= min && node.Score <= max {
			results = append(results, resp.Object{Type: resp.BulkStringPrefix, Bulk: []byte(node.Member)})
		}
	}
	return resp.Object{Type: resp.ArrayPrefix, Array: results}.ToBytes(), nil
}