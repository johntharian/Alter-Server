# Implementation Plan: Interactive Terminal Chat Client

## Overview
We need a way for developers to open two terminal windows, authenticate as User A and User B respectively, and manually chat with each other to test the BotsApp HTTP+WebSocket infrastructure end-to-end. We will build a small interactive CLI client in Go.

## Requirements
- Authenticate via OTP and store JWT
- Sync contacts to discover the other user
- Connect to the WebSocket `/ws/feed` endpoint to receive messages in real-time
- Wait for keyboard input to send a message via `POST /messages` to a selected contact
- Display received messages clearly in the terminal

## Architecture Changes
No changes to the core API server are needed. We will add a new Go binary under `cmd/chatclient/main.go`. We will reuse the `dto` structs.

## Implementation Steps

### Phase 1: Authentication and Profile
1. **Interactive Auth Flow** (File: `cmd/chatclient/main.go`)
   - Action: Prompt user for phone number.
   - Action: Call `POST /auth/otp/request`.
   - Action: Prompt user for OTP code.
   - Action: Call `POST /auth/otp/verify`.
   - Action: Store JWT in memory.

### Phase 2: Contact Sync and Selection
2. **Setup Chat Session** (File: `cmd/chatclient/main.go`)
   - Action: Prompt user to enter the phone number of the person they want to chat with.
   - Action: Call `POST /contacts/sync` to ensure they are discovered.
   - Action: Find the contact's [id](file:///Users/john/Desktop/john/projects/botsapp/internal/auth/jwt.go#68-99) from the sync response.

### Phase 3: WebSocket and Messaging Loop
3. **Receive Updates via WebSocket** (File: `cmd/chatclient/main.go`)
   - Action: Dial `ws://localhost:8080/ws/feed?token=...`
   - Action: Run a goroutine reading from the WebSocket.
   - Action: When a `new_message` event arrives, print it formatting nicely (e.g., `[Friend]: Hello!`).
   - Action: When a `status_update` event arrives, display [(Delivered)](file:///Users/john/Desktop/john/projects/botsapp/internal/redis/redis.go#51-54) etc.

4. **Send Updates via CLI** (File: `cmd/chatclient/main.go`)
   - Action: Read lines from `bufio.Scanner(os.Stdin)`.
   - Action: On enter, send payload via `POST /messages` to the selected contact.
   - Action: Display `[You]: <message text> (Queued)`.

## Testing Strategy
- E2E: Open two terminal tabs. Run `go run cmd/chatclient/main.go` in both. Log in as `+919999900001` and `+919999900002`. Chat between them.

## Success Criteria
- [ ] User can authenticate via terminal natively
- [ ] User receives messages instantly via WebSocket
- [ ] User sends messages by typing and hitting Enter
