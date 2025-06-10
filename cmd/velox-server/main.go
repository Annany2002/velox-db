package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
)

func handleConnection(conn net.Conn) {
	defer conn.Close()
	log.Printf("Client connected: %s", conn.RemoteAddr())

	reader := bufio.NewReader(conn)

	for {
		// Read a line of input from the client.
		rawCommand, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				log.Printf("Error reading from client: %s", err.Error())
			}
			return
		}

		// NOTE: This is a temporary, non-compliant parser.
		// It splits by spaces and doesn't handle RESP arrays or bulk strings correctly.
		// It will be replaced by a real RESP parser in a future step.
		parts := strings.Fields(strings.TrimSpace(rawCommand))
		if len(parts) == 0 {
			continue
		}

		command := strings.ToUpper(parts[0])
		args := parts[1:]

		// Command router
		switch command {
		case "PING":
			conn.Write([]byte("+PONG\r\n"))
		case "SET":
			if len(args) != 2 {
				conn.Write([]byte("-ERR wrong number of arguments for 'set' command\r\n"))
				continue
			}
			key, value := args[0], args[1]
			mu.Lock() // Acquire an exclusive lock for writing
			data[key] = []byte(value)
			mu.Unlock() // Release the lock
			conn.Write([]byte("+OK\r\n"))
		case "GET":
			if len(args) != 1 {
				conn.Write([]byte("-ERR wrong number of arguments for 'get' command\r\n"))
				continue
			}
			key := args[0]
			mu.RLock() // Acquire a shared lock for reading
			value, ok := data[key]
			mu.RUnlock() // Release the lock

			if !ok {
				conn.Write([]byte("$-1\r\n")) // RESP Null Bulk String
			} else {
				// RESP Bulk String format: $<length>\r\n<data>\r\n
				resp := fmt.Sprintf("$%d\r\n%s\r\n", len(value), value)
				conn.Write([]byte(resp))
			}
		case "DEL":
			if len(args) != 1 {
				conn.Write([]byte("-ERR wrong number of arguments for 'del' command\r\n"))
				continue
			}
			key := args[0]
			mu.Lock()
			_, ok := data[key]
			delete(data, key)
			mu.Unlock()

			if ok {
				conn.Write([]byte(":1\r\n")) // RESP Integer: 1 for success
			} else {
				conn.Write([]byte(":0\r\n")) // RESP Integer: 0 if key didn't exist
			}
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