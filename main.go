package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

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

	// Read IPFS gateway URL from environment
	ipfsGatewayURL := os.Getenv("IPFS_GATEWAY_URL")
	if ipfsGatewayURL == "" {
		ipfsGatewayURL = "https://dweb.link/ipfs/"
	}
	// Ensure it ends with /
	if !strings.HasSuffix(ipfsGatewayURL, "/") {
		ipfsGatewayURL += "/"
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
		_, err := storeBlobInIPFS(ctx, ipfsShell, sqlDB, sha256, ext, body)
		return err
	})

	// Set up LoadBlob handler
	bl.LoadBlob = append(bl.LoadBlob, func(ctx context.Context, sha256 string, ext string) (io.ReadSeeker, error) {
		return loadBlobFromIPFS(ctx, ipfsShell, sqlDB, sha256, ext)
	})

	// Wrap the relay with middleware to modify blossom responses
	handler := modifyBlossomResponse(relay, sqlDB, ipfsGatewayURL)

	// Add healthcheck endpoint
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthCheckHandler(sqlDB, ipfsShell))
	mux.Handle("/", handler)

	log.Printf("Running blossom server on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// modifyBlossomResponse wraps the relay to intercept and modify blossom JSON responses
func modifyBlossomResponse(relay http.Handler, db *sql.DB, gatewayURL string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if this is a GET request to a blob URL (pattern: /sha256.ext)
		if r.Method == "GET" {
			path := strings.TrimPrefix(r.URL.Path, "/")

			// Check if path looks like a blob (has a dot, suggesting a file extension)
			if strings.Contains(path, ".") {
				// Extract SHA256 (everything before the last dot)
				lastDot := strings.LastIndex(path, ".")
				if lastDot > 0 {
					sha256 := path[:lastDot]
					ext := path[lastDot:]

					// SHA256 should be 64 hex characters
					if len(sha256) == 64 {
						// Look up CID from database
						var ipfsCID string
						query := `SELECT ipfs_cid FROM ipfs_blossom_mapping WHERE sha256 = ?`
						err := db.QueryRow(query, sha256).Scan(&ipfsCID)
						if err == nil && ipfsCID != "" {
							// Build gateway URL with filename
							filename := "file" + ext
							gatewayURLWithFile := gatewayURL + ipfsCID + "?filename=" + url.QueryEscape(filename)

							// Redirect to IPFS gateway
							log.Printf("DEBUG: Redirecting blob request sha256=%s to IPFS gateway: %s", sha256, gatewayURLWithFile)
							http.Redirect(w, r, gatewayURLWithFile, http.StatusFound)
							return
						}
					}
				}
			}
		}

		// Use a response writer that captures the response
		capturedWriter := &responseCapturer{
			ResponseWriter: w,
			statusCode:     200,
			body:           &bytes.Buffer{},
			headers:        make(http.Header),
		}

		// Call the original handler
		relay.ServeHTTP(capturedWriter, r)

		// Check if this is a JSON response (check status and body content)
		bodyBytes := capturedWriter.body.Bytes()
		bodyStr := string(bodyBytes)

		// Debug logging for troubleshooting
		if strings.Contains(bodyStr, `"sha256"`) {
			sha256Count := strings.Count(bodyStr, `"sha256"`)
			hasNewlines := strings.Contains(bodyStr, "\n") || strings.Contains(bodyStr, "\r\n")
			log.Printf("DEBUG: Response with sha256 detected - status=%d, len=%d, sha256Count=%d, hasNewlines=%v, path=%s",
				capturedWriter.statusCode, len(bodyBytes), sha256Count, hasNewlines, r.URL.Path)
		}

		if capturedWriter.statusCode >= 200 && capturedWriter.statusCode < 300 && len(bodyBytes) > 0 {
			// Check if this is a JSON array (list command format)
			// The response might be a JSON array with newlines or NDJSON
			hasSha256 := strings.Contains(bodyStr, `"sha256"`)
			sha256Count := strings.Count(bodyStr, `"sha256"`)

			// Check if it's a JSON array (starts with [)
			if hasSha256 && len(bodyBytes) > 0 && bodyBytes[0] == '[' {
				log.Printf("DEBUG: Detected JSON array format with %d sha256 fields", sha256Count)
				// Try to parse as JSON array
				var responseArray []map[string]interface{}
				if err := json.Unmarshal(bodyBytes, &responseArray); err == nil && len(responseArray) > 0 {
					log.Printf("DEBUG: Successfully parsed as JSON array with %d items", len(responseArray))
					modified := false
					for i := range responseArray {
						if sha256, ok := responseArray[i]["sha256"].(string); ok {
							log.Printf("DEBUG: Looking up CID for sha256=%s", sha256)
							// Look up CID from database
							var ipfsCID, ext string
							query := `SELECT ipfs_cid, extension FROM ipfs_blossom_mapping WHERE sha256 = ?`
							err := db.QueryRow(query, sha256).Scan(&ipfsCID, &ext)
							if err != nil {
								log.Printf("DEBUG: Database lookup failed for sha256=%s: %v", sha256, err)
							} else if ipfsCID != "" {
								log.Printf("DEBUG: Found CID=%s, ext=%s for sha256=%s", ipfsCID, ext, sha256)
								// Add CID to response
								responseArray[i]["cid"] = ipfsCID

								// Build gateway URL with filename
								filename := ""
								if ext != "" {
									filename = "file" + ext
								}
								gatewayURLWithFile := gatewayURL + ipfsCID
								if filename != "" {
									gatewayURLWithFile += "?filename=" + url.QueryEscape(filename)
								}
								// Replace the url field with the gateway URL
								responseArray[i]["url"] = gatewayURLWithFile
								modified = true
							}
						}
					}

					if modified {
						// Copy headers from captured response
						for key, values := range capturedWriter.headers {
							for _, value := range values {
								w.Header().Add(key, value)
							}
						}
						// Write modified response
						w.WriteHeader(capturedWriter.statusCode)
						json.NewEncoder(w).Encode(responseArray)
						return
					}
				}
			}

			// Check if this is newline-delimited JSON (NDJSON) - alternative list command format
			hasNewlines := strings.Contains(bodyStr, "\n") || strings.Contains(bodyStr, "\r\n")

			log.Printf("DEBUG: NDJSON check - hasNewlines=%v, hasSha256=%v, sha256Count=%d", hasNewlines, hasSha256, sha256Count)

			if (hasNewlines || sha256Count > 1) && hasSha256 && bodyBytes[0] != '[' {
				log.Printf("DEBUG: Processing as NDJSON, body length=%d", len(bodyStr))
				log.Printf("DEBUG: Full original response body:\n%s", bodyStr)
				log.Printf("DEBUG: Response body bytes (hex): %x", bodyBytes)
				// This is NDJSON format (list command)
				// Normalize line endings and split
				normalizedBody := strings.ReplaceAll(bodyStr, "\r\n", "\n")
				// Split by newline and filter out empty lines
				allLines := strings.Split(normalizedBody, "\n")
				var lines []string
				for _, l := range allLines {
					trimmed := strings.TrimSpace(l)
					if trimmed != "" {
						lines = append(lines, trimmed)
					}
				}
				log.Printf("DEBUG: Found %d non-empty lines", len(lines))
				var modifiedLines []string

				for i, line := range lines {
					line = strings.TrimSpace(line)
					// Skip empty lines
					if line == "" {
						log.Printf("DEBUG: Line %d is empty, skipping", i)
						continue
					}

					// Skip lines that don't look like JSON objects
					if !strings.HasPrefix(line, "{") || !strings.HasSuffix(line, "}") {
						log.Printf("DEBUG: Line %d doesn't look like JSON (first 50 chars): %s", i, line[:min(50, len(line))])
						continue
					}

					log.Printf("DEBUG: Attempting to parse line %d (length=%d): %s...", i, len(line), line[:min(100, len(line))])
					var item map[string]interface{}
					if err := json.Unmarshal([]byte(line), &item); err != nil {
						log.Printf("DEBUG: Failed to unmarshal line %d: %v, full line: %s", i, err, line)
						// Don't add failed lines to output - skip them
						continue
					}

					if sha256, ok := item["sha256"].(string); ok {
						log.Printf("DEBUG: Looking up CID for sha256=%s", sha256)
						// Look up CID from database
						var ipfsCID, ext string
						query := `SELECT ipfs_cid, extension FROM ipfs_blossom_mapping WHERE sha256 = ?`
						err := db.QueryRow(query, sha256).Scan(&ipfsCID, &ext)
						if err != nil {
							log.Printf("DEBUG: Database lookup failed for sha256=%s: %v", sha256, err)
						} else if ipfsCID != "" {
							log.Printf("DEBUG: Found CID=%s, ext=%s for sha256=%s", ipfsCID, ext, sha256)
							// Add CID to response
							item["cid"] = ipfsCID

							// Build gateway URL with filename
							filename := ""
							if ext != "" {
								filename = "file" + ext
							}
							gatewayURLWithFile := gatewayURL + ipfsCID
							if filename != "" {
								gatewayURLWithFile += "?filename=" + url.QueryEscape(filename)
							}
							// Replace the url field with the gateway URL
							item["url"] = gatewayURLWithFile
							log.Printf("DEBUG: Replaced URL with gateway URL: %s", gatewayURLWithFile)
						} else {
							log.Printf("DEBUG: CID is empty for sha256=%s", sha256)
						}
					}
					// Re-encode the modified item
					modifiedJSON, err := json.Marshal(item)
					if err == nil {
						modifiedLines = append(modifiedLines, string(modifiedJSON))
					} else {
						log.Printf("DEBUG: Failed to marshal modified item: %v", err)
						modifiedLines = append(modifiedLines, line)
					}
				}

				// Always write the response if we processed it (even if no modifications)
				if len(modifiedLines) > 0 {
					log.Printf("DEBUG: Writing modified response with %d lines", len(modifiedLines))
					// Copy headers from captured response
					for key, values := range capturedWriter.headers {
						for _, value := range values {
							w.Header().Add(key, value)
						}
					}
					// Write modified NDJSON response
					w.WriteHeader(capturedWriter.statusCode)
					modifiedBody := []byte(strings.Join(modifiedLines, "\n") + "\n")
					w.Write(modifiedBody)
					log.Printf("DEBUG: Wrote %d bytes of modified response", len(modifiedBody))
					return
				} else {
					log.Printf("DEBUG: No modified lines to write")
				}
			}

			// Try to parse as array (alternative list response format)
			var responseArray []map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &responseArray); err == nil && len(responseArray) > 0 {
				// This is an array response (list command)
				modified := false
				for i := range responseArray {
					if sha256, ok := responseArray[i]["sha256"].(string); ok {
						// Look up CID from database
						var ipfsCID, ext string
						query := `SELECT ipfs_cid, extension FROM ipfs_blossom_mapping WHERE sha256 = ?`
						err := db.QueryRow(query, sha256).Scan(&ipfsCID, &ext)
						if err == nil && ipfsCID != "" {
							// Add CID to response
							responseArray[i]["cid"] = ipfsCID

							// Build gateway URL with filename
							filename := ""
							if ext != "" {
								filename = "file" + ext
							}
							gatewayURLWithFile := gatewayURL + ipfsCID
							if filename != "" {
								gatewayURLWithFile += "?filename=" + url.QueryEscape(filename)
							}
							// Replace the url field with the gateway URL
							responseArray[i]["url"] = gatewayURLWithFile
							modified = true
						}
					}
				}
				if modified {
					// Copy headers from captured response
					for key, values := range capturedWriter.headers {
						for _, value := range values {
							w.Header().Add(key, value)
						}
					}
					// Write modified response
					w.WriteHeader(capturedWriter.statusCode)
					json.NewEncoder(w).Encode(responseArray)
					return
				}
			}

			// Try to parse as single object (upload response)
			// Only try if it looks like JSON (starts with {)
			if len(bodyBytes) > 0 && bodyBytes[0] == '{' {
				var responseData map[string]interface{}
				if err := json.Unmarshal(bodyBytes, &responseData); err == nil {
					// Extract SHA256 from response if available (indicates blossom upload)
					if sha256, ok := responseData["sha256"].(string); ok {
						// Look up CID from database
						var ipfsCID, ext string
						query := `SELECT ipfs_cid, extension FROM ipfs_blossom_mapping WHERE sha256 = ?`
						err := db.QueryRow(query, sha256).Scan(&ipfsCID, &ext)
						if err == nil && ipfsCID != "" {
							// Add CID to response
							responseData["cid"] = ipfsCID

							// Build gateway URL with filename
							filename := ""
							if ext != "" {
								filename = "file" + ext
							}
							gatewayURLWithFile := gatewayURL + ipfsCID
							if filename != "" {
								gatewayURLWithFile += "?filename=" + url.QueryEscape(filename)
							}
							// Replace the url field with the gateway URL
							responseData["url"] = gatewayURLWithFile

							// Copy headers from captured response
							for key, values := range capturedWriter.headers {
								for _, value := range values {
									w.Header().Add(key, value)
								}
							}
							// Write modified response
							w.WriteHeader(capturedWriter.statusCode)
							json.NewEncoder(w).Encode(responseData)
							return
						}
					}
				}
			}
		}

		// If modification failed or not applicable, write original response
		// Copy headers
		for key, values := range capturedWriter.headers {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(capturedWriter.statusCode)
		w.Write(capturedWriter.body.Bytes())
	})
}

// responseCapturer captures the response for modification
type responseCapturer struct {
	http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
	headers    http.Header
}

func (rc *responseCapturer) Header() http.Header {
	return rc.headers
}

func (rc *responseCapturer) WriteHeader(code int) {
	rc.statusCode = code
}

func (rc *responseCapturer) Write(b []byte) (int, error) {
	// Capture all writes to the body
	return rc.body.Write(b)
}

func (rc *responseCapturer) Flush() {
	// Implement Flush if needed
	if flusher, ok := rc.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
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
// Returns the CID for use in response modification
func storeBlobInIPFS(ctx context.Context, ipfsShell *shell.Shell, db *sql.DB, sha256 string, ext string, body []byte) (string, error) {
	log.Printf("Storing blob: sha256=%s, ext=%s, size=%d", sha256, ext, len(body))

	// Upload to IPFS
	reader := bytes.NewReader(body)
	cid, err := ipfsShell.Add(reader)
	if err != nil {
		return "", fmt.Errorf("failed to upload to IPFS: %w", err)
	}

	log.Printf("Uploaded to IPFS: sha256=%s -> cid=%s", sha256, cid)

	// Store mapping in database
	query := `INSERT OR REPLACE INTO ipfs_blossom_mapping (sha256, ipfs_cid, extension) VALUES (?, ?, ?)`
	_, err = db.ExecContext(ctx, query, sha256, cid, ext)
	if err != nil {
		return "", fmt.Errorf("failed to store mapping: %w", err)
	}

	return cid, nil
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

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// healthCheckHandler returns a health check endpoint handler
func healthCheckHandler(db *sql.DB, ipfsShell *shell.Shell) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := "healthy"
		statusCode := http.StatusOK
		checks := make(map[string]interface{})

		// Check database connectivity
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			status = "unhealthy"
			statusCode = http.StatusServiceUnavailable
			checks["database"] = map[string]interface{}{
				"status": "unhealthy",
				"error":  err.Error(),
			}
		} else {
			checks["database"] = map[string]interface{}{
				"status": "healthy",
			}
		}

		// Check IPFS connectivity
		if ipfsShell != nil {
			if ipfsShell.IsUp() {
				checks["ipfs"] = map[string]interface{}{
					"status": "healthy",
				}
			} else {
				status = "unhealthy"
				statusCode = http.StatusServiceUnavailable
				checks["ipfs"] = map[string]interface{}{
					"status": "unhealthy",
					"error":  "IPFS API is not accessible",
				}
			}
		}

		// Get memory stats
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		checks["memory"] = map[string]interface{}{
			"alloc_mb":       m.Alloc / 1024 / 1024,
			"total_alloc_mb": m.TotalAlloc / 1024 / 1024,
			"sys_mb":         m.Sys / 1024 / 1024,
			"num_gc":         m.NumGC,
		}

		// Get goroutine count
		checks["goroutines"] = runtime.NumGoroutine()

		response := map[string]interface{}{
			"status":    status,
			"checks":    checks,
			"timestamp": time.Now().Unix(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(response)
	}
}
