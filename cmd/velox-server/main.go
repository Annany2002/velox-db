package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"strings"
)

func handleConnection(conn net.Conn) {
	defer conn.Close()
	log.Printf("Client connected: %s", conn.RemoteAddr())

	// Create a new RESPReader for this connection
	respReader := NewRESPReader(conn)

	for {
		// Use the new parser to read a complete command object
		obj, err := respReader.ReadObject()
		if err != nil {
			if err != io.EOF {
				log.Printf("Error reading command: %s", err.Error())
			}
			return
		}

		// Commands are sent as an Array of Bulk Strings
		if obj.Type != ArrayPrefix || len(obj.Array) == 0 {
			conn.Write([]byte("-ERR invalid command format\r\n"))
			continue
		}

		// Extract command and arguments
		commandObj := obj.Array[0]
		if commandObj.Type != BulkStringPrefix {
			conn.Write([]byte("-ERR command must be a bulk string\r\n"))
			continue
		}
		command := strings.ToUpper(string(commandObj.Bulk))
		args := obj.Array[1:]

		// Command router
		switch command {
		case "PING":
			conn.Write([]byte("+PONG\r\n"))
		case "SET":
			if len(args) != 2 || args[0].Type != BulkStringPrefix || args[1].Type != BulkStringPrefix {
				conn.Write([]byte("-ERR wrong number or type of arguments for 'set' command\r\n"))
				continue
			}
			key, value := string(args[0].Bulk), args[1].Bulk
			mu.Lock()
			data[key] = value
			mu.Unlock()
			conn.Write([]byte("+OK\r\n"))
		case "GET":
			if len(args) != 1 || args[0].Type != BulkStringPrefix {
				conn.Write([]byte("-ERR wrong number or type of arguments for 'get' command\r\n"))
				continue
			}
			key := string(args[0].Bulk)
			mu.RLock()
			value, ok := data[key]
			mu.RUnlock()

			if !ok {
				conn.Write([]byte("$-1\r\n"))
			} else {
				resp := fmt.Sprintf("$%d\r\n%s\r\n", len(value), value)
				conn.Write([]byte(resp))
			}
		case "DEL":
			if len(args) < 1 {
				conn.Write([]byte("-ERR wrong number of arguments for 'del' command\r\n"))
				continue
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
			conn.Write([]byte(fmt.Sprintf(":%d\r\n", deletedCount)))
		default:
			conn.Write([]byte(fmt.Sprintf("-ERR unknown command '%s'\r\n", command)))
		}
	}
}

func main() {
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
		go handleConnection(conn)
	}
}