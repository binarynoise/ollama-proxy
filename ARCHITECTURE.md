# Ollama Proxy - High-Level Design Document

## Purpose

A reverse proxy with a live terminal UI that intercepts and displays AI API calls (Ollama, OpenAI, Claude) for observability and debugging.

---

## Core Concept

The system acts as a transparent intermediary between AI clients and an upstream AI API server. It captures request/response payloads for specific endpoints and presents them in a real-time interactive console view.

---

## Architecture Overview

```
┌─────────────┐      ┌─────────────────┐      ┌─────────────────┐
│   Client    │◄────►│  Reverse Proxy  │◄────►│  Upstream API   │
│  (any AI    │      │  (with          │      │  (Ollama/etc)   │
│   client)   │      │   Interceptor)  │      │                 │
└─────────────┘      └────────┬────────┘      └─────────────────┘
                              │
                              ▼
                    ┌─────────────────┐
                    │   Call Store    │
                    │  (In-Memory     │
                    │   Database)     │
                    └────────┬────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │  Terminal UI    │
                    │  (Live View)    │
                    └─────────────────┘
```

---

## Component Specifications

### 1. Reverse Proxy

**Responsibility:** Forward HTTP requests to an upstream server while optionally intercepting specific endpoints.

**Behavior:**

- Accepts HTTP requests on a configurable listen address
- Modifies incoming requests to target the upstream server (URL rewriting)
- Routes all traffic to the upstream API server and back
- Delegates to an Interceptor for capture-worthy requests
- Forwards errors and cleanly shuts the affected connections

**Configuration:**

- Listen address (host:port)
- Target upstream URL

---

### 2. Interceptor

**Responsibility:** Decide which requests to capture and extract full request/response payloads.

**Endpoint Filtering:**
Intercept ONLY these endpoints (path suffix matching):

- `/api/chat` (Ollama chat)
- `/api/generate` (Ollama generate)
- `/v1/chat/completions` (OpenAI chat)
- `/v1/completions` (OpenAI completions)
- `/v1/messages` (Claude messages)

All other requests pass through unmodified (and are only logged, not captured).

**Request Capture:**

- Read the complete request body
- Create a record in the Call Store

**Response Capture:**
The response writer wrapper must:

- Buffer streaming data
- Handle chunked responses (SSE or jsonl)
- Parse the format: lines can contain complete JSON objects (jsonl) or be prefixed with `data:` containing JSON payloads (SSE)
- Extract complete JSON objects from potentially fragmented chunks

**Error Handling:**

- Mark calls as errored on HTTP 4xx/5xx responses or upstream connection errors
- Mark calls as disconnected when client closes connection

---

### 3. Call Store

**Responsibility:** Maintain a bounded, in-memory history of API calls with event streaming.

**Data Model - Call:**

- Unique identifier (UUID)
- HTTP method (GET, POST, etc.)
- Endpoint path
- Status: ACTIVE | DONE | ERROR | DISCONNECTED
- Start timestamp
- End timestamp (optional)
- Request body (string)
- Response body (string, append as data arrives)

**Behavior:**

- Creates new Call records on intercepted requests
- Supports concurrent updates to response content (thread-safe)
- Enforces maximum capacity by removing the oldest calls
- Maintains an event stream for real-time UI updates

**Event Model:**
Events notify subscribers of changes:

- Published on: new call, response update, call completion, call error, call disconnected

---

### 4. Terminal UI

**Responsibility:** Display captured calls and their details in an interactive console interface.

**Layout:**

- Left panel: Scrollable list of recent calls
  - Columns: Short ID, Status Icon, Method, Endpoint, Duration
  - Status icons: Active (🟢), Done (✅), Error (❌), Disconnected (🟠)
  - Scroll position stays at the top (newest calls appear at the top) unless manually scrolled down
- Right panel: Detail view for selected call
  - Model name, Prompt/Messages, Response (concatenated from streaming chunks)
  - Auto-scrolls to show latest response content unless user has manually scrolled up
- Bottom panel: Log output from the application

**Interaction:**

- Navigation: Up/Down arrows move through call list
- Selection: Enter or automatic selection on navigation
- Focus: Tab/Shift-Tab cycles between panels
- Quit: Press 'q' to exit

**Formatting by Endpoint Type:**

*Ollama /api/generate:*

- Display: Model name, Prompt, Response (concatenated from streaming chunks)

*Ollama /api/chat:*

- Display: Model name, Messages (role + content per message), Response

*OpenAI /v1/chat/completions:*

- Display: Model name, Messages, Response

*OpenAI /v1/completions:*

- Display: Model name, Prompt, Response

*Claude /v1/messages:*

- Display: Model name, Messages, Response

**Color Coding:**

- Model names: Highlighted
- Roles (User/Assistant/System): Highlighted
- Prompt/Request: Highlighted
- Response: Normal text
- Reasoning/Thinking: Dimmed/subdued

**Real-time Updates:**

- Refresh call list on new/updated calls
- Update detail view when selected call is updated or new data arrives
- Auto-scroll to latest content in response panel

---

## Data Flow

### Request Flow

1. Client sends HTTP request to Proxy
2. Proxy sets up communication between client and upstream
3. Proxy sends a copy of the request to the Call Store
4. Call Store creates a new Call record with the request data
5. Proxy sends a streamed copy of the (streamed) response to the Call Buffer as it is received
6. Call Buffer buffers the response chunks until valid JSON is ready to be parsed
7. Call Buffer sends the parsed JSON to the Call Store
8. Call Store extracts relevant fields and updates the Call record

### UI Update Flow

1. UI subscribes to Call Store Events channel
2. On each event: Refresh call list display
3. If event is for selected call: Refresh detail view
4. Detail formatting parses JSON based on endpoint type

---

## Concurrency Considerations

- Multiple requests may be in-flight simultaneously
- Call Store must be thread-safe for concurrent reads/writes
- Response streaming updates must not block proxy forwarding
- UI updates must be queued/dispatched to avoid blocking event processing
- Context cancellation must propagate for clean shutdown

---

## Error Handling - Special Cases

**Proxy Connection Errors:**

- Log error, mark associated call as errored
- Return HTTP 502 Bad Gateway to client

**Client Disconnection:**

- Detect early termination of the client connection
- Mark call as DISCONNECTED
- Close upstream connection to stop processing response

**Unknown JSON content:**

- Log warning, dump raw content into response pane for debugging

---

## Shutdown Behavior

On shutdown signal (SIGINT/SIGTERM or internal shutdown request):

1. Cancel context to stop accepting new connections
2. Allow in-flight requests to complete (with timeout)
3. Close UI event processing
4. Exit cleanly

---

## Configuration Parameters

| Parameter      | Default                  | Description                      |
| -------------- | ------------------------ | -------------------------------- |
| Listen Address | `:11444`                 | Host:port to listen on           |
| Target URL     | `http://localhost:11434` | Upstream API server              |
| Max Calls      | `50`                     | Maximum calls to keep in history |

---

## Dependencies (for reference)

- Full-window Terminal UI library (for rendering the interface)
- UUID generation (for unique call IDs)
- JSON parsing (streaming-friendly)
- HTTP reverse proxy functionality
- Context management for cancellation

---

## Testing Considerations

The system should handle:

- Concurrent requests
- Large request/response bodies
- Streaming responses with varying chunk sizes (including edge cases like empty chunks, JSON split across chunks)
- Client disconnections mid-request
- Server errors (4xx/5xx)
- Graceful shutdown under load

Unit tests should cover parsing of different endpoint response formats (chat, generate, messages, SSE, etc.; streamed and non-streamed)
