# 🚀 VeloxDB

VeloxDB is a high-performance, in-memory database built with Go, inspired by Redis. This project aims to replicate core Redis functionalities while exploring modern Go features for concurrency and performance.

## 🎯 Current Status: MVP Development

We are currently building the Minimum Viable Product (MVP). The goal is to create a stable core that can be extended in the future.

### MVP Scope
- A TCP server that handles multiple concurrent clients.
- A simplified command parser.
- In-memory key-value storage (data is ephemeral).
- Support for core commands: `PING`, `SET`, `GET`, `DEL`.

## ⚙️ How to Run

1.  Navigate to the server directory:
    ```sh
    cd cmd/velox-server
    ```
2.  Run the server:
    ```sh
    go run main.go
    ```
3.  The server will start listening on `localhost:6380`.