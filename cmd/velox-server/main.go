package main

import (
	"log"

	"github.com/Annany2002/velox-db/internal/aof"
	"github.com/Annany2002/velox-db/internal/server"
	"github.com/Annany2002/velox-db/internal/store"
)

func main() {
	const aofPath = "velox-db.aof"
	addr := "localhost:6380"

	// 1. Create a new store
	s := store.New()

	// 2. Load from AOF into the store
	log.Println("Loading data from AOF file...")
	cmdChan, err := aof.Load(aofPath)
	if err != nil {
		log.Fatalf("Failed to load AOF file: %v", err)
	}
	if cmdChan != nil {
		// Use a temporary server instance just for applying commands during load
		tempServer := server.New(s, nil)
		for cmd := range cmdChan {
			if err := tempServer.ApplyCommandForLoad(cmd); err != nil {
				log.Printf("Error applying command from AOF: %s", err.Error())
			}
		}
		log.Println("AOF data loaded successfully.")
	}

	// 3. Create the AOF manager
	aofManager, err := aof.New(aofPath)
	if err != nil {
		log.Fatalf("Failed to initialize AOF: %v", err)
	}
	defer aofManager.Close()

	// 4. Create and start the main server
	srv := server.New(s, aofManager)
	if err := srv.Start(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}