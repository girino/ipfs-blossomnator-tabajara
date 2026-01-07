# IPFS Blossomnator Tabajara

A Blossom server implementation built using [Khatru](https://github.com/fiatjaf/khatru), a flexible Nostr relay framework. This server stores blob files in IPFS while maintaining full compatibility with the Blossom protocol. Built on Khatru, it uses SQLite3 for event storage and automatically redirects blob requests to IPFS gateway URLs.

## Features

- **IPFS Backend**: All blob files are stored in IPFS via HTTP API
- **SQLite3 Storage**: Uses SQLite3 for event storage and metadata mapping
- **Automatic Redirects**: Blob GET requests automatically redirect to IPFS gateway URLs
- **Enhanced JSON Responses**: Upload and list responses include IPFS CID and gateway URLs
- **Upload Authorization**: Optional pubkey whitelist to restrict uploads to authorized users
- **Docker Support**: Fully containerized with Docker Compose for easy deployment

## Quick Start with Docker (Recommended)

The easiest way to run IPFS Blossomnator Tabajara is using Docker Compose, which sets up both the Blossom server and a local IPFS node.

### Prerequisites

- Docker and Docker Compose installed
- At least 2GB of free disk space for IPFS data

### Running with Docker Compose

**Important**: This project provides two Docker Compose files:
- `docker-compose.prod.yml` - Production setup with healthchecks and auto-restart
- `docker-compose.dev.yml` - Development setup (simpler, no healthchecks)

**You must copy one of these files to `docker-compose.yml` before running.**

**Before running, choose one of the following:**

#### Option 1: Use Production Setup (Recommended for Production)

1. Copy the production file:
```bash
cp docker-compose.prod.yml docker-compose.yml
```

2. Edit `docker-compose.yml` if needed (e.g., to use pre-built images)

3. Start the services:
```bash
docker-compose up -d
```

#### Option 2: Use Development Setup

1. Copy the development file:
```bash
cp docker-compose.dev.yml docker-compose.yml
```

2. Start the services:
```bash
docker-compose up -d
```

#### Quick Start (Using Production Setup)

1. Clone the repository:
```bash
git clone <repository-url>
cd ipfs-blossomnator-tabajara
```

2. Copy and use the production compose file:
```bash
cp docker-compose.prod.yml docker-compose.yml
```

3. Start the services:
```bash
docker-compose up -d
```

This will:
- Start a local IPFS node
- Build and start the Blossom server
- Set up healthchecks for both services
- Enable autoheal to automatically restart unhealthy containers
- Create `./ipfs-data` and `./blossom-data` directories for persistent storage

4. Check the logs:
```bash
docker-compose logs -f
```

5. Check health status:
```bash
# Check container health
docker-compose ps

# Check health endpoint directly
curl http://localhost:3334/health
```

6. Stop the services:
```bash
docker-compose down
```

### Health Checks

The server includes a `/health` endpoint that reports:
- **Database connectivity**: Checks SQLite database access
- **IPFS connectivity**: Verifies IPFS API is accessible
- **Memory usage**: Reports allocated, total allocated, and system memory, with threshold checking
- **Goroutines**: Current number of goroutines, with threshold checking
- **GC stats**: Garbage collection statistics

The healthcheck will mark the service as unhealthy if:
- Memory usage exceeds `HEALTHCHECK_MAX_MEMORY_MB` (default: 512 MB)
- Goroutine count exceeds `HEALTHCHECK_MAX_GOROUTINES` (default: 1000)

Example health check response (healthy):
```json
{
  "status": "healthy",
  "checks": {
    "database": {"status": "healthy"},
    "ipfs": {"status": "healthy"},
    "memory": {
      "status": "healthy",
      "alloc_mb": 15,
      "total_alloc_mb": 25,
      "sys_mb": 50,
      "num_gc": 2,
      "max_mb": 512
    },
    "goroutines": {
      "status": "healthy",
      "count": 12,
      "max": 1000
    },
    "timestamp": 1704067200
  }
}
```

Example health check response (unhealthy due to thresholds):
```json
{
  "status": "unhealthy",
  "checks": {
    "database": {"status": "healthy"},
    "ipfs": {"status": "healthy"},
    "memory": {
      "status": "unhealthy",
      "alloc_mb": 600,
      "max_mb": 512,
      "error": "Memory usage 600 MB exceeds threshold 512 MB"
    },
    "goroutines": {
      "status": "unhealthy",
      "count": 1500,
      "max": 1000,
      "error": "Goroutine count 1500 exceeds threshold 1000"
    },
    "timestamp": 1704067200
  }
}
```

The production Docker Compose setup uses this endpoint for container healthchecks and auto-restart via autoheal.

### Environment Variables

You can customize the configuration using environment variables:

```bash
# Set custom port (default: 3334)
export PORT=8080

# Set custom IPFS gateway URL (default: https://dweb.link/ipfs/)
export IPFS_GATEWAY_URL=https://ipfs.io/ipfs/

# Set allowed pubkeys for upload authorization (comma-separated, npub or hex format)
export ALLOWED_PUBKEYS="npub1abc...,npub2def...,0123456789abcdef..."

# Start with custom settings
docker-compose up -d
```

Or edit `docker-compose.yml` directly to set environment variables.

## Manual Installation

If you prefer to run without Docker:

### Prerequisites

- Go 1.25 or later
- SQLite3 development libraries
- Access to an IPFS node (local or remote)

### Building

```bash
go build -o blossom-server main.go
```

### Running

1. Set required environment variables:
```bash
export IPFS_API_URL=http://localhost:5001
export PORT=3334
export DATABASE_PATH=./blossom.db
export IPFS_GATEWAY_URL=https://dweb.link/ipfs/
```

2. Run the server:
```bash
./blossom-server
```

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `IPFS_API_URL` | Yes | - | IPFS HTTP API endpoint (e.g., `http://localhost:5001`) |
| `PORT` | No | `3334` | Server port to listen on |
| `DATABASE_PATH` | No | `./blossom.db` | Path to SQLite database file |
| `IPFS_GATEWAY_URL` | No | `https://dweb.link/ipfs/` | Public IPFS gateway URL for redirects |
| `ALLOWED_PUBKEYS` | No | - | Comma-separated list of allowed pubkeys for uploads (npub or hex format). If not set, uploads are unrestricted. Downloads are always unrestricted. |
| `HEALTHCHECK_MAX_MEMORY_MB` | No | `512` | Maximum memory usage in MB before marking unhealthy |
| `HEALTHCHECK_MAX_GOROUTINES` | No | `1000` | Maximum number of goroutines before marking unhealthy |

### IPFS Setup

If you're running IPFS locally, make sure:

1. IPFS is installed and running
2. The API is accessible at the URL specified in `IPFS_API_URL`
3. The IPFS node is properly initialized

For Docker Compose, the IPFS node is automatically configured.

## Upload Authorization

The server supports optional upload authorization via a pubkey whitelist. When `ALLOWED_PUBKEYS` is set, only authenticated users with pubkeys in the whitelist can upload blobs. Downloads are always unrestricted.

### Setting Up Authorization

1. **Get your pubkey** (in npub or hex format):
   - npub format: `npub1abc...` (Bech32 encoded)
   - hex format: `0123456789abcdef...` (64 hex characters)

2. **Set the whitelist** via environment variable:
   ```bash
   export ALLOWED_PUBKEYS="npub1abc...,npub2def...,0123456789abcdef..."
   ```

   Or in `docker-compose.yml`:
   ```yaml
   environment:
     ALLOWED_PUBKEYS: "npub1abc...,npub2def..."
   ```

3. **Restart the server** to apply changes.

### How It Works

- **Uploads**: When `ALLOWED_PUBKEYS` is set, uploads require NIP-98 HTTP authentication, and the pubkey from the auth event must be in the whitelist. Unauthorized uploads return HTTP 403.
- **Downloads**: Downloads are always unrestricted and do not require authentication.
- **Format Support**: The whitelist accepts both npub (Bech32) and hex formats. All keys are normalized to hex for comparison.

### Example

```bash
# Allow only specific pubkeys to upload
export ALLOWED_PUBKEYS="npub1abc123...,npub1def456..."

# Start server
./blossom-server

# Uploads from whitelisted pubkeys: ✅ Allowed
# Uploads from other pubkeys: ❌ Rejected (403 Forbidden)
# Downloads: ✅ Always allowed (no auth required)
```

## API Usage

### Upload a Blob

```bash
nak blossom upload -server localhost:3334 image.jpg
```

Response includes:
- `url`: IPFS gateway URL (replaces local URL)
- `sha256`: SHA256 hash of the blob
- `cid`: IPFS Content Identifier
- `size`: File size in bytes
- `type`: MIME type
- `uploaded`: Unix timestamp

### List Blobs

```bash
nak blossom list -server localhost:3334
```

Returns an array of blob objects, each with:
- `url`: IPFS gateway URL
- `sha256`: SHA256 hash
- `cid`: IPFS Content Identifier
- `size`: File size
- `type`: MIME type
- `uploaded`: Unix timestamp

### Access a Blob

When you access a blob URL like:
```
http://localhost:3334/f21e5746d1efac1bddb87a630a2f6b093c3f0151716857bc387fdc44ff65319a.jpg
```

The server automatically redirects to the IPFS gateway:
```
https://dweb.link/ipfs/Qm...?filename=file.jpg
```

## Architecture

This implementation is built on [Khatru](https://github.com/fiatjaf/khatru), a flexible and extensible Nostr relay framework written in Go. Khatru provides the core relay functionality, while this project extends it with:

- **Blossom Extension**: Uses Khatru's Blossom extension for blob storage
- **IPFS Backend**: Custom storage handlers that integrate with IPFS
- **SQLite3 Event Store**: Uses `github.com/fiatjaf/eventstore/sqlite3` for event persistence
- **Response Modification**: Middleware to enhance responses with IPFS gateway URLs

```
┌─────────────┐
│   Client    │
└──────┬──────┘
       │
       ▼
┌─────────────────┐
│  Blossom Server │
└──────┬──────────┘
       │
       ├──► SQLite3 (Events + Metadata)
       │
       └──► IPFS API (Blob Storage)
              │
              ▼
         ┌────────┐
         │  IPFS  │
         └────────┘
```

### Data Flow

1. **Upload**: Blob is uploaded to IPFS, CID is stored in SQLite mapping table
2. **List**: Server queries SQLite for all blobs, looks up CIDs, and returns IPFS gateway URLs
3. **Get**: Server looks up CID from SQLite and redirects to IPFS gateway

## Database Schema

The server creates a mapping table in SQLite:

```sql
CREATE TABLE ipfs_blossom_mapping (
    sha256 TEXT PRIMARY KEY,
    ipfs_cid TEXT NOT NULL,
    extension TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

## Development

### Building

```bash
go build -o blossom-server main.go
```

### Testing

1. Start a local IPFS node:
```bash
ipfs daemon
```

2. Run the server:
```bash
export IPFS_API_URL=http://localhost:5001
./blossom-server
```

3. Test with `nak`:
```bash
nak blossom upload -server localhost:3334 test.jpg
nak blossom list -server localhost:3334
```

## Docker Images

Pre-built Docker images are automatically built and published to GitHub Container Registry when you create a semantic version tag (e.g., `v1.0.0`, `v2.1.3`).

### Using Pre-built Images

To use a pre-built image instead of building from source, edit `docker-compose.yml`:

```yaml
services:
  blossom:
    # Comment out the build section and use image instead
    # build:
    #   context: .
    #   dockerfile: Dockerfile
    image: ghcr.io/girino/ipfs-blossomnator-tabajara:v1.0.0
    container_name: blossom-server
    # ... rest of configuration
```


### Available Tags

Images are tagged with:
- `v1.0.0` - Full semantic version
- `1.0.0` - Version without 'v' prefix
- `1.0` - Major.minor version
- `1` - Major version only

### Pulling Images Manually

```bash
docker pull ghcr.io/girino/ipfs-blossomnator-tabajara:v1.0.0
```

## Troubleshooting

### IPFS Connection Issues

If you see "IPFS API at ... is not accessible":
- Ensure IPFS is running
- Check that `IPFS_API_URL` points to the correct endpoint
- Verify IPFS API is accessible (default: `http://localhost:5001`)

### Database Issues

If you encounter database errors:
- Check that the directory for `DATABASE_PATH` is writable
- Ensure SQLite3 libraries are installed
- Check disk space availability

### Port Conflicts

If the port is already in use:
- Change the `PORT` environment variable
- Update port mappings in `docker-compose.yml`

## License

This project is licensed under [Girino's Anarchist License (GAL)](https://license.girino.org).

## Contributing

[Add contribution guidelines here]

