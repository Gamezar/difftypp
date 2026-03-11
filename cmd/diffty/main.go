package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/Gamezar/difftypp/internal/server"
	"github.com/Gamezar/difftypp/internal/storage"
	"github.com/Gamezar/difftypp/internal/updater"
)

var version = "dev"

func main() {
	// Command line flags
	port := flag.Int("port", 10101, "Port to run the server on")
	showVersion := flag.Bool("version", false, "Print diffty++ version")
	updateBinary := flag.Bool("update", false, "Update diffty++ to the latest GitHub release")
	flag.Parse()

	if *showVersion {
		fmt.Printf("diffty++ %s\n", version)
		return
	}

	if *updateBinary {
		tag, updated, err := updater.SelfUpdate(version)
		if err != nil {
			log.Fatalf("Failed to update diffty++: %v", err)
		}
		if !updated {
			fmt.Printf("diffty++ is already up to date (%s)\n", tag)
			return
		}
		fmt.Printf("Updated diffty++ to %s\n", tag)
		return
	}

	// Initialize storage for review state
	store, err := storage.NewJSONStorage()
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}

	// Setup server and routes
	srv, err := server.New(store)
	if err != nil {
		log.Fatalf("Failed to initialize server: %v", err)
	}

	// Start server
	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Starting diffty++ server at http://localhost%s", addr)

	if err := http.ListenAndServe(addr, srv.Router()); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
