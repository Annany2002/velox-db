package main

import (
	"bufio"
	"container/list"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
)

// WRONGTYPE_ERROR is a standard Redis error message for operating on the wrong key type.
const WRONGTYPE_ERROR = "WRONGTYPE Operation against a key holding the wrong kind of value"

// applyCommand now handles type checking and list commands.
func applyCommand(obj RESPObject, aof *Aof) ([]byte, error) {
	if obj.Type != ArrayPrefix || len(obj.Array) == 0 {
		return nil, fmt.Errorf("invalid command format: not an array")
	}

	commandObj := obj.Array[0]
	if commandObj.Type != BulkStringPrefix {
		return nil, fmt.Errorf("invalid command format: command is not a bulk string")
	}
	command := strings.ToUpper(string(commandObj.Bulk))
	args := obj.Array[1:]

	switch command {
	case "PING":
		return []byte("+PONG\r\n"), nil
	case "SET":
		if len(args) != 2 || args[0].Type != BulkStringPrefix || args[1].Type != BulkStringPrefix {
			return nil, fmt.Errorf("ERR wrong number or type of arguments for 'set' command")
		}
		key, value := string(args[0].Bulk), args[1].Bulk
		mu.Lock()
		data[key] = []byte(value)
		mu.Unlock()
		return []byte("+OK\r\n"), nil
	case "GET":
		if len(args) != 1 || args[0].Type != BulkStringPrefix {
			return nil, fmt.Errorf("ERR wrong number or type of arguments for 'get' command")
		}
		key := string(args[0].Bulk)
		mu.RLock()
		rawValue, ok := data[key]
		mu.RUnlock()
		if !ok {
			return []byte("$-1\r\n"), nil
		}
		// Type assertion to check if the value is a string
		value, ok := rawValue.([]byte)
		if !ok {
			return nil, fmt.Errorf(WRONGTYPE_ERROR)
		}
		return []byte(fmt.Sprintf("$%d\r\n%s\r\n", len(value), value)), nil
	case "DEL":
		if len(args) < 1 {
			return nil, fmt.Errorf("ERR wrong number of arguments for 'del' command")
		}
		var deletedCount int
		mu.Lock()
		for _, arg := range args {
			if arg.Type != BulkStringPrefix { continue }
			key := string(arg.Bulk)
			if _, ok := data[key]; ok {
				delete(data, key)
				deletedCount++
			}
		}
		mu.Unlock()
		return []byte(fmt.Sprintf(":%d\r\n", deletedCount)), nil
	
	// NEW LIST COMMANDS
	case "LPUSH", "RPUSH":
		if len(args) < 2 {
			return nil, fmt.Errorf("ERR wrong number of arguments for '%s' command", strings.ToLower(command))
		}
		key := string(args[0].Bulk)
		values := args[1:]
		mu.Lock()
		defer mu.Unlock()

		rawValue, ok := data[key]
		if !ok {
			// If the key doesn't exist, create a new list
			rawValue = list.New()
			data[key] = rawValue
		}
		
		listValue, ok := rawValue.(*list.List)
		if !ok {
			return nil, fmt.Errorf(WRONGTYPE_ERROR)
		}

		for _, v := range values {
			if command == "LPUSH" {
				listValue.PushFront(v.Bulk)
			} else {
				listValue.PushBack(v.Bulk)
			}
		}
		return fmt.Appendf(nil, ":%d\r\n", listValue.Len()), nil

	case "LPOP", "RPOP":
		if len(args) != 1 {
			return nil, fmt.Errorf("ERR wrong number of arguments for '%s' command", strings.ToLower(command))
		}
		key := string(args[0].Bulk)
		mu.Lock()
		defer mu.Unlock()
		
		rawValue, ok := data[key]
		if !ok {
			return []byte("$-1\r\n"), nil // Key doesn't exist
		}
		listValue, ok := rawValue.(*list.List)
		if !ok {
			return nil, fmt.Errorf(WRONGTYPE_ERROR)
		}
		if listValue.Len() == 0 {
			return []byte("$-1\r\n"), nil // List is empty
		}

		var element *list.Element
		if command == "LPOP" {
			element = listValue.Front()
		} else {
			element = listValue.Back()
		}
		
		listValue.Remove(element)
		// If the list is now empty, remove it from the map to free memory
		if listValue.Len() == 0 {
			delete(data, key)
		}

		value := element.Value.([]byte)
		return fmt.Appendf(nil, "$%d\r\n%s\r\n", len(value), value), nil
	
	// NEW READ-ONLY LIST COMMANDS
	case "LLEN":
		if len(args) != 1 {
			return nil, fmt.Errorf("ERR wrong number of arguments for 'llen' command")
		}
		key := string(args[0].Bulk)
		mu.RLock()
		defer mu.RUnlock()

		rawValue, ok := data[key]
		if !ok {
			return []byte(":0\r\n"), nil // Key doesn't exist, length is 0
		}
		listValue, ok := rawValue.(*list.List)
		if !ok {
			return nil, fmt.Errorf(WRONGTYPE_ERROR)
		}
		return fmt.Appendf(nil, ":%d\r\n", listValue.Len()), nil

	case "LINDEX":
		if len(args) != 2 {
			return nil, fmt.Errorf("ERR wrong number of arguments for 'lindex' command")
		}
		key := string(args[0].Bulk)
		index, err := strconv.Atoi(string(args[1].Bulk))
		if err != nil {
			return nil, fmt.Errorf("ERR value is not an integer or out of range")
		}

		mu.RLock()
		defer mu.RUnlock()

		rawValue, ok := data[key]
		if !ok {
			return []byte("$-1\r\n"), nil
		}
		listValue, ok := rawValue.(*list.List)
		if !ok {
			return nil, fmt.Errorf(WRONGTYPE_ERROR)
		}

		// Handle negative index
		if index < 0 {
			index = listValue.Len() + index
		}

		if index < 0 || index >= listValue.Len() {
			return []byte("$-1\r\n"), nil // Index out of bounds
		}

		// Traverse the list to find the element
		i := 0
		for e := listValue.Front(); e != nil; e = e.Next() {
			if i == index {
				value := e.Value.([]byte)
				return fmt.Appendf(nil, "$%d\r\n%s\r\n", len(value), value), nil
			}
			i++
		}
		return []byte("$-1\r\n"), nil // Should be unreachable, but good practice

	case "LRANGE":
		if len(args) != 3 {
			return nil, fmt.Errorf("ERR wrong number of arguments for 'lrange' command")
		}
		key := string(args[0].Bulk)
		start, err1 := strconv.Atoi(string(args[1].Bulk))
		stop, err2 := strconv.Atoi(string(args[2].Bulk))
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("ERR value is not an integer or out of range")
		}

		mu.RLock()
		defer mu.RUnlock()

		rawValue, ok := data[key]
		if !ok {
			return []byte("*0\r\n"), nil // Return empty array if key doesn't exist
		}
		listValue, ok := rawValue.(*list.List)
		if !ok {
			return nil, fmt.Errorf(WRONGTYPE_ERROR)
		}

		// Normalize negative indices
		listLen := listValue.Len()
		if start < 0 {
			start = listLen + start
		}
		if stop < 0 {
			stop = listLen + stop
		}
		// Clamp indices to list bounds
		if start < 0 {
			start = 0
		}
		if stop >= listLen {
			stop = listLen - 1
		}

		var results []RESPObject
		if start > stop || start >= listLen {
			return []byte("*0\r\n"), nil // Return empty array for invalid range
		}
		
		i := 0
		for e := listValue.Front(); e != nil && i <= stop; e = e.Next() {
			if i >= start {
				results = append(results, RESPObject{Type: BulkStringPrefix, Bulk: e.Value.([]byte)})
			}
			i++
		}

		return RESPObject{Type: ArrayPrefix, Array: results}.ToBytes(), nil

	case "REWRITEAOF":
		if err := aof.Rewrite(); err != nil {
			return nil, fmt.Errorf("ERR failed to rewrite AOF: %v", err)
		}
		return []byte("+OK\r\n"), nil
	default:
		return nil, fmt.Errorf("ERR unknown command '%s'", command)
	}
}

func handleConnection(conn net.Conn, aof *Aof) {
	defer conn.Close()
	log.Printf("Client connected: %s", conn.RemoteAddr())

	respReader := NewRESPReader(conn)

	for {
		obj, err := respReader.ReadObject()
		if err != nil {
			if err != io.EOF {
				log.Printf("Error reading command from client: %s", err.Error())
			}
			return
		}

		// Pass the aof instance to applyCommand
		response, err := applyCommand(obj, aof)
		if err != nil {
			conn.Write([]byte(fmt.Sprintf("-%s\r\n", err.Error())))
			continue
		}

		// This simple check is still valid for identifying write commands
		command := strings.ToUpper(string(obj.Array[0].Bulk))
		if command == "SET" || command == "DEL" || command == "LPUSH" || command == "RPUSH" || command == "LPOP" || command == "RPOP"{
			if err := aof.Write(obj); err != nil {
				log.Printf("Failed to write to AOF: %s", err.Error())
			}
		}

		conn.Write(response)
	}
}

func loadAof(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) { return nil }
		return err
	}
	defer file.Close()

	log.Println("Loading data from AOF file...")
	reader := NewRESPReader(bufio.NewReader(file))
	for {
		obj, err := reader.ReadObject()
		if err != nil {
			if err == io.EOF { break }
			return err
		}
		if _, err := applyCommand(obj, nil); err != nil {
			log.Printf("Error applying command from AOF: %s", err.Error())
		}
	}
	log.Println("AOF data loaded successfully.")
	return nil
}

func main() {
	aofPath := "velox-db.aof"

	if err := loadAof(aofPath); err != nil {
		log.Fatalf("Failed to load data from AOF: %s", err.Error())
	}

	aof, err := NewAof(aofPath)
	if err != nil {
		log.Fatalf("Failed to initialize AOF: %s", err.Error())
	}
	defer aof.Close()

	addr := "localhost:6380"
	log.Printf("VeloxDB server starting on %s", addr)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to bind to address %s: %s", addr, err.Error())
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %s", err.Error())
			continue
		}
		go handleConnection(conn, aof)
	}
}