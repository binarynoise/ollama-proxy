# Ollama Proxy

Ollama Proxy is a small reverse proxy and terminal UI (TUI) that helps observe requests to an Ollama server.
It forwards traffic to an upstream Ollama instance while recording request/response payloads and surfacing them in a live console view.

## Features

- Reverse proxy that forwards requests to an Ollama API server
- Request interception for `/api/chat` and `/api/generate`, capturing payloads
- Call tracker that keeps a bounded history with live updates
- Terminal UI showing:
  - List of recent calls with status and duration
  - Request/response details formatted for chat and generate endpoints

## Requirements

- Go 1.24+
- Access to an Ollama server (defaults to `http://localhost:11434`)

## Getting Started

Point your client at the proxy (e.g., `http://localhost:11444`) instead of the Ollama server directly.
Calls will appear in the TUI with their status, payload, and streaming output.

### Development

```bash
go run ./cmd/ollama-proxy-tui
```

### Build & Run

```bash
go build ./cmd/ollama-proxy-tui

./ollama-proxy-tui
# or with flags
./ollama-proxy-tui \
  -listen :11444 \
  -target http://localhost:11434 \
  -max-calls 50
```

Flags:

- `-listen`: address the proxy listens on (default `:11444`)
- `-target`: URL of the upstream Ollama API (default `http://localhost:11434`)
- `-max-calls`: maximum number of calls kept in history (default `50`)

## Project Structure

- `cmd/ollama-proxy-tui`: entrypoint that starts the proxy and TUI
- `internal/proxy`: reverse proxy and interception logic
- `internal/tracker`: in-memory call tracker and event stream
- `internal/tui`: terminal UI built with `tview`
- `internal/types`: shared call/event types
