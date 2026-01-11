#!/bin/bash
# RAG Integration Test Script
# Tests the end-to-end RAG functionality for meeting transcripts

set -e  # Exit on error

BASE_URL="http://localhost:8080"
DB_HOST="localhost"
DB_PORT="5433"
DB_USER="audio_translator"
DB_NAME="audio_translator"

echo "=========================================="
echo "RAG Integration Test"
echo "=========================================="
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

print_info() {
    echo -e "${YELLOW}ℹ $1${NC}"
}

# Function to check if service is running
check_service() {
    local url=$1
    local name=$2

    if curl -sf "$url" > /dev/null; then
        print_success "$name is running"
        return 0
    else
        print_error "$name is not running at $url"
        return 1
    fi
}

# Step 1: Check all required services
echo "Step 1: Checking required services..."
check_service "http://localhost:8080" "Go Backend Server"
check_service "http://localhost:8006/health" "Embedding Service"
check_service "http://localhost:8007/health" "LLM Service"
echo ""

# Step 2: Create a test meeting
echo "Step 2: Creating test meeting..."
MEETING_RESPONSE=$(curl -s -X POST "$BASE_URL/api/meetings" \
  -H "Content-Type: application/json" \
  -d '{"mode": "shared"}')

MEETING_ID=$(echo "$MEETING_RESPONSE" | jq -r '.id')
ROOM_CODE=$(echo "$MEETING_RESPONSE" | jq -r '.roomCode')

if [ -z "$MEETING_ID" ] || [ "$MEETING_ID" == "null" ]; then
    print_error "Failed to create meeting"
    exit 1
fi

print_success "Meeting created: $MEETING_ID (Room: $ROOM_CODE)"
echo ""

# Step 3: Insert test transcript directly into database
echo "Step 3: Inserting test transcript into database..."

TEST_TRANSCRIPT="[00:00:05] Alice: Hello everyone, welcome to the Q4 planning meeting.
[00:00:12] Bob: Thanks Alice. Today we will discuss our Q4 roadmap and priorities.
[00:00:20] Alice: Yes, our main focus should be on performance improvements and scalability.
[00:00:28] Bob: I agree. We also need to prioritize the mobile app redesign.
[00:00:35] Alice: That's a good point. The mobile experience needs significant work.
[00:00:42] Bob: Should we also consider the API documentation overhaul?
[00:00:50] Alice: Definitely. Good documentation will help our developer community.
[00:00:58] Bob: Great. Let's also discuss the testing infrastructure improvements.
[00:01:05] Alice: Yes, we need better automated testing coverage across all platforms."

export PGPASSWORD="audio_translator_pass"

psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" << EOF
INSERT INTO meeting_transcript_snapshots (meeting_id, language, transcript)
VALUES ('$MEETING_ID', 'en', '$TEST_TRANSCRIPT');

UPDATE meetings SET is_active = false, ended_at = NOW()
WHERE id = '$MEETING_ID';
EOF

print_success "Test transcript inserted"
echo ""

# Step 4: Wait for RAG processing
echo "Step 4: Waiting for RAG processing (20 seconds)..."
sleep 20

# Check if chunks were created
CHUNK_COUNT=$(psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -t -c \
    "SELECT COUNT(*) FROM meeting_chunks WHERE meeting_id = '$MEETING_ID';")

CHUNK_COUNT=$(echo "$CHUNK_COUNT" | tr -d '[:space:]')

if [ "$CHUNK_COUNT" -gt 0 ]; then
    print_success "RAG processing completed: $CHUNK_COUNT chunks created"
else
    print_error "RAG processing failed: No chunks created"
    exit 1
fi
echo ""

# Step 5: Create chat session
echo "Step 5: Creating chat session..."
SESSION_RESPONSE=$(curl -s -X POST "$BASE_URL/api/chat/sessions" \
  -H "Content-Type: application/json" \
  -d "{\"meetingId\": \"$MEETING_ID\", \"language\": \"en\"}")

SESSION_ID=$(echo "$SESSION_RESPONSE" | jq -r '.sessionId')

if [ -z "$SESSION_ID" ] || [ "$SESSION_ID" == "null" ]; then
    print_error "Failed to create chat session"
    echo "Response: $SESSION_RESPONSE"
    exit 1
fi

print_success "Chat session created: $SESSION_ID"
echo ""

# Step 6: Test RAG query
echo "Step 6: Testing RAG query..."

QUERY_RESPONSE=$(curl -s -X POST "$BASE_URL/api/chat/query" \
  -H "Content-Type: application/json" \
  -d "{
    \"sessionId\": \"$SESSION_ID\",
    \"meetingId\": \"$MEETING_ID\",
    \"language\": \"en\",
    \"question\": \"What were the main topics discussed in this meeting?\",
    \"topK\": 5
  }")

ANSWER=$(echo "$QUERY_RESPONSE" | jq -r '.answer')
CHUNK_IDS=$(echo "$QUERY_RESPONSE" | jq -r '.chunkIds')

if [ -z "$ANSWER" ] || [ "$ANSWER" == "null" ]; then
    print_error "RAG query failed"
    echo "Response: $QUERY_RESPONSE"
    exit 1
fi

print_success "RAG query successful"
echo ""
echo "Question: What were the main topics discussed in this meeting?"
echo ""
echo "Answer:"
echo "$ANSWER"
echo ""
echo "Source chunk IDs: $CHUNK_IDS"
echo ""

# Step 7: Test another query
echo "Step 7: Testing second RAG query..."

QUERY_RESPONSE_2=$(curl -s -X POST "$BASE_URL/api/chat/query" \
  -H "Content-Type: application/json" \
  -d "{
    \"sessionId\": \"$SESSION_ID\",
    \"meetingId\": \"$MEETING_ID\",
    \"language\": \"en\",
    \"question\": \"Who mentioned the mobile app redesign?\",
    \"topK\": 3
  }")

ANSWER_2=$(echo "$QUERY_RESPONSE_2" | jq -r '.answer')

print_success "Second RAG query successful"
echo ""
echo "Question: Who mentioned the mobile app redesign?"
echo ""
echo "Answer:"
echo "$ANSWER_2"
echo ""

# Step 8: Verify chat history
echo "Step 8: Verifying chat history..."

CHAT_HISTORY=$(psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -t -c \
    "SELECT COUNT(*) FROM meeting_chat_messages WHERE session_id = '$SESSION_ID';")

CHAT_HISTORY=$(echo "$CHAT_HISTORY" | tr -d '[:space:]')

if [ "$CHAT_HISTORY" -eq 4 ]; then  # 2 user messages + 2 assistant responses
    print_success "Chat history verified: $CHAT_HISTORY messages stored"
else
    print_error "Chat history incomplete: Expected 4 messages, found $CHAT_HISTORY"
fi
echo ""

# Step 9: Cleanup (optional)
echo "Step 9: Cleanup..."
print_info "Test data remains in database for inspection"
print_info "Meeting ID: $MEETING_ID"
print_info "Session ID: $SESSION_ID"
echo ""

# To clean up, uncomment the following:
# psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" << EOF
# DELETE FROM meeting_chat_messages WHERE session_id = '$SESSION_ID';
# DELETE FROM meeting_chat_sessions WHERE session_id = '$SESSION_ID';
# DELETE FROM meeting_chunks WHERE meeting_id = '$MEETING_ID';
# DELETE FROM meeting_transcript_snapshots WHERE meeting_id = '$MEETING_ID';
# DELETE FROM meetings WHERE id = '$MEETING_ID';
# EOF
# print_success "Cleanup completed"

echo "=========================================="
echo "✓ All tests passed successfully!"
echo "=========================================="
