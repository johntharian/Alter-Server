#!/bin/bash
# End-to-end test script for Alter Phase 1
# Prerequisites: API server running on :8080, all infrastructure up

set -e

BASE="http://localhost:8080"
PHONE_A="+919999900001"
PHONE_B="+919999900002"

echo "=========================================="
echo "Alter E2E Test"
echo "=========================================="

# --- 1. Health Check ---
echo -e "\n[1] Health check..."
curl -sf "$BASE/health" | jq .

# --- 2. Signup User A ---
echo -e "\n[2] Request OTP for User A ($PHONE_A)..."
curl -sf -X POST "$BASE/auth/otp/request" \
  -H "Content-Type: application/json" \
  -d "{\"phone_number\": \"$PHONE_A\"}" | jq .

echo "Check API server logs for OTP code, then paste it below:"
read -p "OTP for User A: " OTP_A

echo "Verifying OTP..."
RESULT_A=$(curl -sf -X POST "$BASE/auth/otp/verify" \
  -H "Content-Type: application/json" \
  -d "{\"phone_number\": \"$PHONE_A\", \"code\": \"$OTP_A\"}")
echo "$RESULT_A" | jq .

TOKEN_A=$(echo "$RESULT_A" | jq -r '.token')
USER_A_ID=$(echo "$RESULT_A" | jq -r '.user.id')
echo "User A ID: $USER_A_ID"
echo "Token A: ${TOKEN_A:0:20}..."

# --- 3. Signup User B ---
echo -e "\n[3] Request OTP for User B ($PHONE_B)..."
curl -sf -X POST "$BASE/auth/otp/request" \
  -H "Content-Type: application/json" \
  -d "{\"phone_number\": \"$PHONE_B\"}" | jq .

read -p "OTP for User B: " OTP_B

RESULT_B=$(curl -sf -X POST "$BASE/auth/otp/verify" \
  -H "Content-Type: application/json" \
  -d "{\"phone_number\": \"$PHONE_B\", \"code\": \"$OTP_B\"}")
echo "$RESULT_B" | jq .

TOKEN_B=$(echo "$RESULT_B" | jq -r '.token')
USER_B_ID=$(echo "$RESULT_B" | jq -r '.user.id')
echo "User B ID: $USER_B_ID"

# --- 4. Register Bot Endpoints ---
echo -e "\n[4] Register bot endpoint for User A..."
curl -sf -X PUT "$BASE/users/me/bot" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN_A" \
  -d '{"url": "https://httpbin.org/post"}' | jq .

echo "Register bot endpoint for User B..."
curl -sf -X PUT "$BASE/users/me/bot" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN_B" \
  -d '{"url": "https://httpbin.org/post"}' | jq .

# --- 5. Contact Sync ---
echo -e "\n[5] User A syncs contacts..."
curl -sf -X POST "$BASE/contacts/sync" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN_A" \
  -d "{\"phone_numbers\": [\"$PHONE_B\"]}" | jq .

# --- 6. Send a Message ---
echo -e "\n[6] User A sends a message to User B..."
SEND_RESULT=$(curl -sf -X POST "$BASE/messages" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN_A" \
  -d "{\"to\": \"$PHONE_B\", \"intent\": \"schedule_meeting\", \"payload\": {\"date\": \"2026-03-15\", \"time\": \"14:00\"}}")
echo "$SEND_RESULT" | jq .

THREAD_ID=$(echo "$SEND_RESULT" | jq -r '.thread_id')
echo "Thread ID: $THREAD_ID"

# --- 7. List Threads ---
echo -e "\n[7] User A's threads..."
curl -sf "$BASE/threads" \
  -H "Authorization: Bearer $TOKEN_A" | jq .

# --- 8. Get Thread Messages ---
echo -e "\n[8] Messages in thread..."
curl -sf "$BASE/threads/$THREAD_ID/messages" \
  -H "Authorization: Bearer $TOKEN_A" | jq .

# --- 9. Human Takeover ---
echo -e "\n[9] User A takes over the thread..."
curl -sf -X POST "$BASE/threads/$THREAD_ID/takeover" \
  -H "Authorization: Bearer $TOKEN_A" | jq .

# --- 10. Send human message ---
echo -e "\n[10] User A sends a manual message..."
curl -sf -X POST "$BASE/messages" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN_A" \
  -d "{\"to\": \"$PHONE_B\", \"intent\": \"manual_reply\", \"payload\": {\"text\": \"Hey, I'm replying manually!\"}}" | jq .

# --- 11. Release Takeover ---
echo -e "\n[11] User A releases thread back to bot..."
curl -sf -X DELETE "$BASE/threads/$THREAD_ID/takeover" \
  -H "Authorization: Bearer $TOKEN_A" | jq .

echo -e "\n=========================================="
echo "E2E Test Complete!"
echo "=========================================="
echo ""
echo "WebSocket test: connect to ws://localhost:8080/ws/feed?token=$TOKEN_A"
echo "Then send a message from User B to watch real-time updates."
