# VeloxDB

VeloxDB is a high-performance, in-memory database built from scratch in Go, inspired by the architecture of Redis. This project implements core Redis functionalities, including full RESP protocol support and AOF data persistence, serving as a deep dive into database internals and concurrent programming in Go.

## Project Status

The project has successfully implemented a robust, persistent key-value store. The core MVP is complete, including full data persistence and recovery via an Append-Only File (AOF). The server is stable, compatible with standard Redis clients, and serves as a strong foundation for future feature development.

### Key Features

- **Concurrent TCP Server**: Handles multiple clients simultaneously using a goroutine-per-connection model.
- **Full RESP Protocol Support**: A compliant parser and serializer for the Redis Serialization Protocol, making it compatible with any Redis client.
- **AOF (Append-Only File) Persistence & Recovery**: All write operations (`SET`, `DEL`) are logged to disk for durability. The database state is automatically restored from the AOF on startup.
- **Concurrent-Safe In-Memory Storage**: A thread-safe data store using `RWMutex` to allow for high-performance concurrent reads.
- **Hot Reload Development**: Configured with Air for efficient, live-reloading development (as implemented by you).

## Project Goals

- To gain a fundamental understanding of how in-memory databases like Redis work.
- To apply concurrent programming patterns in Go to solve real-world problems.
- To implement a network protocol (RESP) from scratch.
- To explore different database persistence strategies, starting with AOF.

## How to Run

### Prerequisites

- Go 1.22 or later
- `redis-cli` (or another Redis client) for testing

### Standard Execution

1.  Clone the repository:
    ```sh
    git clone https://github.com/Annany2002/velox-db
    ```
2.  Navigate to the server's directory:
    ```sh
    cd velox-db/cmd/velox-server
    ```
3.  Run the server (the `.` tells Go to run the current package):
    ```sh
    go run .
    ```

### Development with Hot Reload

1.  Install Air (if not already installed):
    ```sh
    go install github.com/air-verse/air@latest
    ```
2.  From the project's root directory (`velox-db`), start the server with Air:
    ```sh
    air
    ```

The server will start listening on `localhost:6380`.

## Testing the Server

### Using `redis-cli`

In a new terminal, connect to the server:

```sh
redis-cli -p 6380
```

You will see the `127.0.0.1:6380>` prompt.

**Example Session:**

```
127.0.0.1:6380> PING
PONG
127.0.0.1:6380> SET mykey "hello world"
OK
127.0.0.1:6380> GET mykey
"hello world"
127.0.0.1:6380> SET user:1 "John Doe"
OK
127.0.0.1:6380> DEL mykey
(integer) 1
127.0.0.1:6380> GET mykey
(nil)
```

## Architecture

The project is organized into modular components, each with a specific responsibility.

### File Structure

- **`main.go`**: Initializes the server, loads data from the AOF, listens for TCP connections, and dispatches connections to handlers.
- **`aof.go`**: Manages all Append-Only File operations, including writing commands and serializing objects to RESP format.
- **`resp.go`**: Contains the complete implementation of the RESP parser and data structures.
- **`store.go`**: Defines the thread-safe, in-memory `map[string][]byte` data store.
- **`.air.toml`**: Development hot-reload configuration.

### Key Components

#### Concurrency Model

- **Goroutine per Connection**: Each client connection is handled in a dedicated goroutine for high concurrency.
- **Shared State Protection**: The in-memory store is protected by a `sync.RWMutex` (multiple readers, single writer), while the AOF file is protected by a `sync.Mutex` to ensure sequential writes.

#### Persistence Layer (AOF)

- **Durable Writes**: All `SET` and `DEL` commands are written to `velox-db.aof`.
- **Startup Recovery**: The server reads and executes all commands from the AOF file upon startup to rehydrate the in-memory state.
- **RESP Format**: Commands are stored in their native, byte-for-byte RESP format for consistency and simplicity.

#### RESP Parser

- **Streaming Parser**: A `bufio.Reader`-based parser that efficiently reads from the network stream.
- **Unified `RESPObject`**: A single struct represents all five RESP data types, simplifying command processing.
- **Binary Safe**: Correctly handles any binary data within keys or values via RESP Bulk Strings.

## Requirements

- Go 1.22 or later.
