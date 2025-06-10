# 🚀 VeloxDB

VeloxDB is a high-performance, in-memory database built with Go, inspired by Redis. This project implements core Redis functionalities with full RESP protocol support and data persistence features.

## ✅ Current Status: Enhanced Implementation

The project has evolved beyond the initial MVP with significant architectural improvements and additional features!

### ✨ Features Implemented

- **TCP Server**: Robust concurrent server handling multiple clients simultaneously
- **Full RESP Protocol**: Complete Redis Serialization Protocol (RESP) parser and responder
- **AOF Persistence**: Append-Only File for data durability and crash recovery
- **Thread-Safe Storage**: In-memory key-value store with RWMutex for concurrent access
- **Hot Reload Development**: Air configuration for live reload during development
- **Proper Error Handling**: Comprehensive error responses following RESP standards

### 🔧 Supported Commands

- **`PING`** → Returns `+PONG`
- **`SET key value`** → Stores a key-value pair (persisted to AOF)
- **`GET key`** → Retrieves the value for a key
- **`DEL key [key ...]`** → Deletes one or more keys (persisted to AOF)

### 🎯 RESP Protocol Features

- **Full RESP Parsing**: Supports all RESP data types (Simple Strings, Errors, Integers, Bulk Strings, Arrays)
- **Binary Safe**: Handles binary data correctly through bulk strings
- **Redis Compatible**: Commands can be sent using any Redis client
- **Proper Error Responses**: Follows Redis error message conventions

### 💾 Persistence

- **AOF (Append-Only File)**: All write operations (`SET`, `DEL`) are logged to `velox-db.aof`
- **Data Recovery**: Automatic restoration from AOF file on server restart
- **Thread-Safe Writes**: Mutex-protected AOF operations

## ⚙️ How to Run

### Standard Go Run

1. Clone the repository:
   ```sh
   git clone https://github.com/Annany2002/velox-db
   ```
2. Navigate to the server directory:
   ```sh
   cd velox-db/cmd/velox-server
   ```
3. Run the server:
   ```sh
   go run main.go aof.go resp.go store.go
   ```

### Development with Hot Reload

1. Install Air (if not already installed):
   ```sh
   go install github.com/cosmtrek/air@latest
   ```
2. From the project root, start with hot reload:
   ```sh
   air
   ```

The server will start listening on `localhost:6380`.

## 🧪 Testing the Server

### Using Redis CLI

```sh
redis-cli -p 6380
```

### Using Telnet or Netcat

```sh
# Using telnet
telnet localhost 6380

# Using netcat
nc localhost 6380
```

### Example Commands

```
PING
SET mykey hello
GET mykey
SET user:1 "John Doe"
GET user:1
DEL mykey user:1
GET mykey
```

### Using Raw RESP Protocol

```
*1\r\n$4\r\nPING\r\n
*3\r\n$3\r\nSET\r\n$5\r\nmykey\r\n$5\r\nhello\r\n
*2\r\n$3\r\nGET\r\n$5\r\nmykey\r\n
```

## 🏗️ Architecture

### File Structure

- **`main.go`**: TCP server, connection handling, and command routing
- **`resp.go`**: Complete RESP protocol parser implementation
- **`aof.go`**: Append-Only File persistence manager
- **`store.go`**: Thread-safe in-memory storage with RWMutex
- **`.air.toml`**: Development hot-reload configuration

### Key Components

#### RESP Parser

- **RESPReader**: Bufio-based streaming parser
- **RESPObject**: Unified representation of all RESP data types
- **Type Safety**: Proper handling of arrays, bulk strings, and error cases

#### Storage Engine

- **In-Memory Map**: `map[string][]byte` for binary-safe storage
- **RWMutex Protection**: Concurrent reads, exclusive writes
- **Binary Safe Values**: Full support for binary data

#### Persistence Layer

- **AOF Manager**: Thread-safe append-only file operations
- **Command Logging**: Automatic persistence of write operations
- **RESP Serialization**: Commands stored in native RESP format

#### Concurrency Model

- **Goroutine per Connection**: Each client runs in its own goroutine
- **Shared State Protection**: RWMutex for data store, Mutex for AOF
- **Non-blocking Architecture**: Concurrent client handling

## 🚀 Performance Features

- **Zero-Copy Operations**: Direct byte slice handling where possible
- **Efficient Parsing**: Streaming RESP parser with minimal allocations
- **Concurrent Access**: Read-optimized with RWMutex
- **Fast Command Routing**: Direct string comparison switch statements

## 🔄 Development Workflow

The project includes Air configuration for hot reload during development:

- Automatic rebuild on Go file changes
- Excludes test files and temporary directories
- Live reload for rapid iteration

## 📋 Requirements

- Go 1.24.2 or later
- Air (optional, for development hot reload)
