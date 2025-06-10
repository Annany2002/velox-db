package main

import (
	"bufio"
	"io"
	"log"
	"net"
)

// handleConnection is now updated to read from the client in a loop.
func handleConnection(conn net.Conn) {
	// Ensure the connection is closed when the function returns.
	defer conn.Close()
	log.Printf("Client connected: %s", conn.RemoteAddr())

	// Create a new buffered reader for the connection.
	reader := bufio.NewReader(conn)

	// Loop indefinitely to read commands from the client.
	for {
		// Read data until a newline character is encountered.
		// This is a temporary, simple way to read commands.
		command, err := reader.ReadString('\n')
		if err != nil {
			// If it's an End-Of-File error, the client has disconnected.
			if err == io.EOF {
				log.Printf("Clien disconnected: %s", conn.RemoteAddr())
			} else {
				log.Printf("Error reading from client: %s", err.Error())
			}
			// Break the loop to end the connection handling.
			return
		}

		// For now, we just log the received command.
		// We trim the space to remove the trailing newline character.
		log.Printf("Received: %s", command)

		// Send a placeholder response back to the client.
		// We use "+PONG\r\n" which is a valid RESP "Simple String".
		conn.Write([]byte("+PONG\r\n"))
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