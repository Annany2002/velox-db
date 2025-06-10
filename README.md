# 🚀 VeloxDB

VeloxDB is a high-performance, in-memory database built with Go, inspired by Redis. This project aims to replicate core Redis functionalities while exploring modern Go features for concurrency and performance.

## ✅ Current Status: MVP Complete

The Minimum Viable Product (MVP) has been successfully implemented and is fully functional!

### ✨ Features Implemented

- **TCP Server**: Concurrent server handling multiple clients simultaneously
- **Command Parser**: Simple yet effective command parsing (space-separated format)
- **Thread-Safe Storage**: In-memory key-value store with RWMutex for concurrent access
- **RESP Protocol**: Proper Redis Serialization Protocol responses
- **Core Commands**: Full support for essential Redis commands

### 🔧 Supported Commands

- **`PING`** → Returns `+PONG`
- **`SET key value`** → Stores a key-value pair
- **`GET key`** → Retrieves the value for a key
- **`DEL key`** → Deletes a key and its value

## ⚙️ How to Run
1. Clone the repository
    ```sh
    git clone https://github.com/Annany2002/velox-db
    ```
2.  Navigate to the server directory:
    ```sh
    cd velox-db/cmd/velox-server
    ```
3.  Run the server:
    ```sh
    go run main.go
    ```
4.  The server will start listening on `localhost:6380`.

## 🧪 Testing the Server

You can test the server using `telnet` or `nc` (netcat):

```sh
# Using telnet
telnet localhost 6380

# Using netcat
nc localhost 6380
```

Then try these commands:

```
PING
SET mykey hello
GET mykey
DEL mykey
GET mykey
```

## 🏗️ Architecture

- **`main.go`**: TCP server and command handling logic
- **`store.go`**: Thread-safe in-memory storage implementation
- **Concurrency**: Each client connection runs in its own goroutine
- **Safety**: RWMutex ensures thread-safe operations on the data store
