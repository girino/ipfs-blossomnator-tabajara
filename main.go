package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/fiatjaf/eventstore/sqlite3"
	"github.com/fiatjaf/khatru"
	"github.com/fiatjaf/khatru/blossom"
	shell "github.com/ipfs/go-ipfs-api"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	// Read environment variables
	ipfsAPIURL := os.Getenv("IPFS_API_URL")
	if ipfsAPIURL == "" {
		log.Fatal("IPFS_API_URL environment variable is required")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "3334"
	}

	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = "./blossom.db"
	}

	// Initialize SQLite3 backend for event storage
	db := &sqlite3.SQLite3Backend{DatabaseURL: dbPath}
	if err := db.Init(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Initialize khatru relay
	relay := khatru.NewRelay()
	relay.StoreEvent = append(relay.StoreEvent, db.SaveEvent)
	relay.QueryEvents = append(relay.QueryEvents, db.QueryEvents)
	relay.CountEvents = append(relay.CountEvents, db.CountEvents)
	relay.DeleteEvent = append(relay.DeleteEvent, db.DeleteEvent)
	relay.ReplaceEvent = append(relay.ReplaceEvent, db.ReplaceEvent)

	// Initialize IPFS client
	ipfsShell := shell.NewShell(ipfsAPIURL)
	
	// Validate IPFS connection
	if !ipfsShell.IsUp() {
		log.Fatalf("IPFS API at %s is not accessible", ipfsAPIURL)
	}

	// Open database connection for mapping table
	sqlDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("Failed to open database connection: %v", err)
	}
	defer sqlDB.Close()

	// Create mapping table if it doesn't exist
	if err := createMappingTable(sqlDB); err != nil {
		log.Fatalf("Failed to create mapping table: %v", err)
	}

	// Initialize blossom
	serviceURL := fmt.Sprintf("http://localhost:%s", port)
	bl := blossom.New(relay, serviceURL)
	bl.Store = blossom.EventStoreBlobIndexWrapper{Store: db, ServiceURL: bl.ServiceURL}

	// Set up StoreBlob handler
	bl.StoreBlob = append(bl.StoreBlob, func(ctx context.Context, sha256 string, ext string, body []byte) error {
		return storeBlobInIPFS(ctx, ipfsShell, sqlDB, sha256, ext, body)
	})

	// Set up LoadBlob handler
	bl.LoadBlob = append(bl.LoadBlob, func(ctx context.Context, sha256 string, ext string) (io.ReadSeeker, error) {
		return loadBlobFromIPFS(ctx, ipfsShell, sqlDB, sha256, ext)
	})

	log.Printf("Running blossom server on :%s", port)
	if err := http.ListenAndServe(":"+port, relay); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// createMappingTable creates the ipfs_blossom_mapping table if it doesn't exist
func createMappingTable(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS ipfs_blossom_mapping (
		sha256 TEXT PRIMARY KEY,
		ipfs_cid TEXT NOT NULL,
		extension TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`
	_, err := db.Exec(query)
	return err
}

// storeBlobInIPFS uploads a blob to IPFS and stores the mapping in the database
func storeBlobInIPFS(ctx context.Context, ipfsShell *shell.Shell, db *sql.DB, sha256 string, ext string, body []byte) error {
	log.Printf("Storing blob: sha256=%s, ext=%s, size=%d", sha256, ext, len(body))

	// Upload to IPFS
	reader := bytes.NewReader(body)
	cid, err := ipfsShell.Add(reader)
	if err != nil {
		return fmt.Errorf("failed to upload to IPFS: %w", err)
	}

	log.Printf("Uploaded to IPFS: sha256=%s -> cid=%s", sha256, cid)

	// Store mapping in database
	query := `INSERT OR REPLACE INTO ipfs_blossom_mapping (sha256, ipfs_cid, extension) VALUES (?, ?, ?)`
	_, err = db.ExecContext(ctx, query, sha256, cid, ext)
	if err != nil {
		return fmt.Errorf("failed to store mapping: %w", err)
	}

	return nil
}

// loadBlobFromIPFS retrieves a blob from IPFS using the mapping stored in the database
func loadBlobFromIPFS(ctx context.Context, ipfsShell *shell.Shell, db *sql.DB, sha256 string, ext string) (io.ReadSeeker, error) {
	log.Printf("Loading blob: sha256=%s, ext=%s", sha256, ext)

	// Look up IPFS CID from database
	var ipfsCID string
	query := `SELECT ipfs_cid FROM ipfs_blossom_mapping WHERE sha256 = ?`
	err := db.QueryRowContext(ctx, query, sha256).Scan(&ipfsCID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("blob not found: sha256=%s", sha256)
		}
		return nil, fmt.Errorf("failed to query mapping: %w", err)
	}

	// Retrieve from IPFS
	reader, err := ipfsShell.Cat(ipfsCID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve from IPFS (cid=%s): %w", ipfsCID, err)
	}
	defer reader.Close()

	// Read all data into memory to create a ReadSeeker
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read IPFS data: %w", err)
	}

	log.Printf("Retrieved from IPFS: sha256=%s -> cid=%s, size=%d", sha256, ipfsCID, len(data))

	return bytes.NewReader(data), nil
}

