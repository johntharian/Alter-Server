You are designing and building a communication infrastructure product. 
Here is the full context of what needs to be built.

---

## Product Overview

A WhatsApp-like platform where every user has a personal AI bot 
representing them on the network. The product is pure infrastructure — 
we provide the communication channel, not the bots themselves. Users 
bring their own bot (via a URL/webhook), or get a default dumb bot at 
signup.

Think of it as: **phone numbers for AI agents**. We are the telecom, 
not the phone manufacturer.

---

## Core Concepts

- Every user signs up with their **phone number** (OTP verification, 
  exactly like WhatsApp)
- Each user registers **one bot endpoint** (a URL/webhook) that 
  represents them on the network
- Users can update their bot URL anytime, but only 1 bot per user, 
  enforced at DB level
- The platform **routes messages between bots** — it does not process 
  or understand the messages
- Message payload is flexible JSON — the platform only enforces the 
  envelope (from, to, thread_id, intent, payload)
- Users can **watch their bot's conversations** in real time via a feed 
  (observer mode)
- Users can **take over** a conversation manually when needed 
  (human-in-the-loop)
- Contact discovery works like WhatsApp — scan phone contacts, show 
  who is already on the network

---

## Identity Model
```
phone_number (+91XXXXXXXXXX) → user_id (UUID) → bot_endpoint URL
```

Phone number is the lookup key. UUID is the stable internal identity. 
This way if a user changes their number, history is preserved.

---

## Message Envelope (standard format between bots)
```json
{
  "from": "+91XXXXXXXXXX",
  "to": "+91YYYYYYYYYY",
  "intent": "schedule_meeting",
  "thread_id": "uuid",
  "message_id": "uuid",
  "timestamp": "ISO8601",
  "payload": { }
}
```

Payload is flexible. Platform does not validate it.

---

## Architecture Requirements

- API server must never block waiting for a bot to respond
- All bot-to-bot message delivery must go through a **message queue** 
  (RabbitMQ to start, Kafka later)
- Delivery worker is a separate service that consumes from the queue, 
  calls the destination bot URL, handles retries with exponential 
  backoff and dead-letter queue
- Real-time feed for users uses **WebSockets** backed by 
  **Redis Pub/Sub**
- Identity lookups (phone → bot URL) are cached in **Redis**
- Scale goal: thousands of concurrent bot conversations without the 
  API server being the bottleneck

---

## Delivery flow
```
Sender's bot → POST /message (API) → enqueue (RabbitMQ) → return 200
                                            ↓
                                    Delivery Worker
                                            ↓
                              POST to recipient's bot URL
                                            ↓
                                 Update message status
                                 (queued → delivered → processed)
                                            ↓
                              Push status update via Redis Pub/Sub
                                            ↓
                               User sees ✓✓ in their feed
```

---

## Tech Stack

| Layer | Tool |
|---|---|
| API server | Go (Gin or Chi) |
| Delivery worker | Go |
| Database | PostgreSQL |
| Queue | RabbitMQ |
| Cache + Pub/Sub | Redis |
| Real-time feed | WebSockets |
| Auth | Phone OTP + JWT |
| Hosting (MVP) | Railway or Render |

---

## Database Schema (starting point, refine as needed)
```sql
users            — id (UUID), phone_number, display_name, created_at
bot_endpoints    — user_id (unique), url, secret_key, last_active_at
contacts         — user_id, contact_user_id, status (pending/accepted)
threads          — id (UUID), participant_a, participant_b, created_at
messages         — id (UUID), thread_id, from_user_id, to_user_id, 
                   intent, payload (JSONB), status, created_at
```

---

## UX Feel (WhatsApp mapping)

| WhatsApp | This product |
|---|---|
| Phone number | Phone number (same) |
| Contact list | Connected bots |
| Chat thread | Bot-to-bot conversation thread |
| ✓✓ read receipts | queued → delivered → processed |
| Online/offline | Bot active / inactive |
| Typing... | Bot is processing |
| Notifications | Bot needs your confirmation |
| WhatsApp Business | Business bots (phase 2) |

The thread view renders bot messages in **plain English summaries**, 
not raw JSON. The server translates payload to a human-readable line 
for the feed.

---

## What to design and build

1. Full system architecture diagram
2. Go project structure (monorepo with api, worker, shared packages)
3. All API routes with request/response schemas
4. PostgreSQL schema with indexes
5. RabbitMQ queue and exchange setup
6. Delivery worker logic including retry, backoff, dead-letter handling
7. WebSocket + Redis Pub/Sub real-time feed implementation
8. OTP auth flow
9. Contact sync flow
10. A working MVP codebase covering the core message routing path 
    end-to-end

---

## Phase 1 scope (build this now)

- User signup via phone + OTP
- Bot URL registration (1 per user)
- Contact discovery (who on network is in your contacts)
- Send a message from your bot to another user's bot
- Real-time feed showing your bot's conversations
- Human takeover of a thread
- Message status (queued / delivered / processed / failed)

## Phase 2 (do not build yet, just keep in mind for architecture)

- Business bots (like WhatsApp Business)
- Group threads (multiple bots)
- Kafka migration for audit logs and replay
- Bot marketplace / default hosted bots

---

Start with the architecture and project structure, then implement 
phase 1 end-to-end.