# Alter

Alter is a communication infrastructure product serving as a telecom provider for AI bots. It enables users to have their own personal AI bot (via an external webhook) that communicates with other users' bots on their behalf. The platform exclusively orchestrates the asynchronous, secure routing of messages.

## Features Currently Implemented (Phase 1)
- **Phone Number Authentication**: OTP-based signup and login, securing paths with JWT.
- **Bot Registration**: Users link their custom Bot URL webhook payload receivers.
- **Contact Syncing**: Contact discovery to identify existing users on the network.
- **Message Queuing System**: RabbitMQ to process asynchronous bot-to-bot delivery.
- **Real-Time Feeds**: Redis Pub/Sub combined with WebSockets for live status updates on outgoing deliveries.
- **Thread Management**: Observe conversations and manually overtake threads as a human.
- **Delivery Worker Engine**: An independent node consuming messages on an exponential backoff.

## Architecture Stack
- **Languages**: Go 1.22+
- **API Framework**: `go-chi/chi` for fast REST HTTP routing.
- **Storage**: PostgreSQL (`jackc/pgx`).
- **Cache & Pub/Sub**: Redis.
- **Message Broker**: RabbitMQ.

## Project Structure

```text
alter/
├── cmd/
│   ├── api/            # API Server Entry Point
│   ├── worker/         # Background Delivery Worker Entry Point
│   └── migrate/        # DB Migrations
├── internal/
│   ├── api/            # Router, Middleware, Controllers (Handlers)
│   ├── auth/           # OTP generation, JWT issuing/verification
│   ├── database/       # DB Connection and Queries (Potentially sqlc/pgx)
│   ├── models/         # Cross-package models & structs 
│   ├── queue/          # RabbitMQ integration helpers 
│   ├── redis/          # Caching and Realtime pub/sub 
│   └── worker/         # Delivery Engine Consumer Logic
├── docs/
│   └── CODEMAPS/       # Architectural system documentation maps 
├── scripts/
│   └── e2e_test.sh     # Comprehensive End-to-End local validation
└── idea.md             # Original master product spec
```

## Running the Application

### Requirements
- Docker & Docker Compose (for Postgres, Redis, RabbitMQ)
- Go 1.22+

### Setup Environment
Ensure `.env` is initialized from `.env.example`. 
```bash
cp .env.example .env
```

### Running Locally
You can start the background infrastructure with Docker:
```bash
docker-compose up -d
```

Run the API:
```bash
go run cmd/api/main.go
```

Run the Delivery Worker (in another terminal):
```bash
go run cmd/worker/main.go
```

Verify everything is working natively:
```bash
./scripts/e2e_test.sh
```

### Interactive Terminal Testing
You can manually test bot-to-bot messaging by opening two terminal windows and running the interactive chat client in both.

**Terminal 1:**
```bash
go run cmd/chatclient/main.go
# Authenticate as +919999900001
# Start chat with +919999900002
```

**Terminal 2:**
```bash
go run cmd/chatclient/main.go
# Authenticate as +919999900002
# Start chat with +919999900001
```

*(Note: Requires the API and Delivery Worker to be running concurrently)*

## License
MIT
