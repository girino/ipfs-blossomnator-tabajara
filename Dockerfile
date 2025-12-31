# Build stage
FROM golang:1.25-alpine AS builder

# Install build dependencies for CGO (required for sqlite3)
RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application with CGO enabled
RUN CGO_ENABLED=1 GOOS=linux go build -o blossom-server main.go

# Runtime stage
FROM alpine:latest

# Install sqlite runtime libraries and ca-certificates
RUN apk --no-cache add ca-certificates sqlite-libs

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/blossom-server .

# Expose port (default 3334, but configurable via PORT env var)
EXPOSE 3334

# Run the server
CMD ["./blossom-server"]

