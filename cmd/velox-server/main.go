package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
)

// applyCommand now takes an Aof instance to handle the REWRITEAOF command.
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
		data[key] = value
		mu.Unlock()
		return []byte("+OK\r\n"), nil
	case "GET":
		if len(args) != 1 || args[0].Type != BulkStringPrefix {
			return nil, fmt.Errorf("ERR wrong number or type of arguments for 'get' command")
		}
		key := string(args[0].Bulk)
		mu.RLock()
		value, ok := data[key]
		mu.RUnlock()
		if !ok {
			return []byte("$-1\r\n"), nil
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
		return fmt.Appendf(nil, ":%d\r\n", deletedCount), nil
	// New command to trigger AOF rewrite
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
			conn.Write(fmt.Appendf(nil, "-%s\r\n", err.Error()))
			continue
		}

		command := strings.ToUpper(string(obj.Array[0].Bulk))
		if command == "SET" || command == "DEL" {
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
		// Pass nil for aof since we won't be rewriting during load
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