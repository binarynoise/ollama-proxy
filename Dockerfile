# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o /ollama-proxy ./cmd/ollama-proxy-tui

# Final stage
FROM alpine:3.18

# Install CA certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

# Copy the binary from builder
COPY --from=builder /ollama-proxy /usr/local/bin/ollama-proxy

# Default environment variables
ENV LISTEN=:11444
ENV TARGET=http://host.docker.internal:11434
ENV MAX_CALLS=50

# Expose the default port
EXPOSE 11444

# Set the entrypoint
ENTRYPOINT ["ollama-proxy"]

# Default command
CMD ["-listen", "${LISTEN}", "-target", "${TARGET}", "-max-calls", "${MAX_CALLS}"]
