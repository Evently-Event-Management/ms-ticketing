package db_test

import (
	"context"
	"database/sql"
	"ms-ticketing/internal/models"
	"ms-ticketing/internal/tickets/db"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	_ "github.com/uptrace/bun/driver/sqliteshim"
)

func setupTicketCountDB(t *testing.T) (*db.DB, *bun.DB) {
	// Connect to an in-memory SQLite DB for testing
	sqldb, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to connect to in-memory database: %v", err)
	}

	// Create a Bun DB instance
	bunDB := bun.NewDB(sqldb, sqlitedialect.New())

	// Create required tables
	_, err = bunDB.NewCreateTable().Model((*models.TicketCount)(nil)).Exec(context.Background())
	if err != nil {
		t.Fatalf("Failed to create ticket_count table: %v", err)
	}

	// Return test DB
	return &db.DB{Bun: bunDB}, bunDB
}

func TestIncrementTicketCount(t *testing.T) {
	// Set up test DB
	ticketCountDB, bunDB := setupTicketCountDB(t)
	defer bunDB.Close()

	// Test case: Increment new ticket count
	eventID := "event123"
	sessionID := "session456"
	timestamp := time.Now().Truncate(24 * time.Hour)

	// Increment count
	err := ticketCountDB.IncrementTicketCount(eventID, sessionID, timestamp)
	assert.NoError(t, err)

	// Verify count is 1
	var count models.TicketCount
	err = bunDB.NewSelect().
		Model(&count).
		Where("event_id = ?", eventID).
		Where("session_id = ?", sessionID).
		Where("date = ?", timestamp).
		Limit(1).
		Scan(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 1, count.Count)

	// Test case: Increment existing ticket count
	err = ticketCountDB.IncrementTicketCount(eventID, sessionID, timestamp)
	assert.NoError(t, err)

	// Verify count is now 2
	err = bunDB.NewSelect().
		Model(&count).
		Where("event_id = ?", eventID).
		Where("session_id = ?", sessionID).
		Where("date = ?", timestamp).
		Limit(1).
		Scan(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 2, count.Count)
}

func TestGetTicketCountsForEvent(t *testing.T) {
	// Set up test DB
	ticketCountDB, bunDB := setupTicketCountDB(t)
	defer bunDB.Close()

	// Create test data
	eventID := "event123"
	timestamp := time.Now().Truncate(24 * time.Hour)
	yesterday := timestamp.AddDate(0, 0, -1)

	testCounts := []models.TicketCount{
		{
			EventID:   eventID,
			SessionID: "session1",
			Count:     10,
			Date:      timestamp,
		},
		{
			EventID:   eventID,
			SessionID: "session2",
			Count:     15,
			Date:      timestamp,
		},
		{
			EventID:   eventID,
			SessionID: "session1",
			Count:     5,
			Date:      yesterday,
		},
		{
			EventID:   "other_event",
			SessionID: "session3",
			Count:     20,
			Date:      timestamp,
		},
	}

	// Insert test data
	_, err := bunDB.NewInsert().Model(&testCounts).Exec(context.Background())
	assert.NoError(t, err)

	// Test case: Get ticket counts for event
	counts, err := ticketCountDB.GetTicketCountsForEvent(eventID)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(counts))

	// Verify we got the right counts for the event
	eventCount := 0
	for _, c := range counts {
		assert.Equal(t, eventID, c.EventID)
		eventCount += c.Count
	}
	assert.Equal(t, 30, eventCount) // 10 + 15 + 5
}

func TestGetTicketCountsForSession(t *testing.T) {
	// Set up test DB
	ticketCountDB, bunDB := setupTicketCountDB(t)
	defer bunDB.Close()

	// Create test data
	sessionID := "session123"
	timestamp := time.Now().Truncate(24 * time.Hour)
	yesterday := timestamp.AddDate(0, 0, -1)

	testCounts := []models.TicketCount{
		{
			EventID:   "event1",
			SessionID: sessionID,
			Count:     10,
			Date:      timestamp,
		},
		{
			EventID:   "event1",
			SessionID: sessionID,
			Count:     5,
			Date:      yesterday,
		},
		{
			EventID:   "event2",
			SessionID: sessionID,
			Count:     8,
			Date:      timestamp,
		},
		{
			EventID:   "event3",
			SessionID: "other_session",
			Count:     20,
			Date:      timestamp,
		},
	}

	// Insert test data
	_, err := bunDB.NewInsert().Model(&testCounts).Exec(context.Background())
	assert.NoError(t, err)

	// Test case: Get ticket counts for session
	counts, err := ticketCountDB.GetTicketCountsForSession(sessionID)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(counts))

	// Verify we got the right counts for the session
	sessionCount := 0
	for _, c := range counts {
		assert.Equal(t, sessionID, c.SessionID)
		sessionCount += c.Count
	}
	assert.Equal(t, 23, sessionCount) // 10 + 5 + 8
}

func TestGetTotalTicketsCount(t *testing.T) {
	// Set up test DB
	ticketCountDB, bunDB := setupTicketCountDB(t)
	defer bunDB.Close()

	// Create test tickets table
	_, err := bunDB.NewCreateTable().Model((*models.Ticket)(nil)).Exec(context.Background())
	assert.NoError(t, err)

	// Insert test tickets
	tickets := []models.Ticket{
		{TicketID: "t1", OrderID: "o1", SeatID: "s1"},
		{TicketID: "t2", OrderID: "o1", SeatID: "s2"},
		{TicketID: "t3", OrderID: "o2", SeatID: "s3"},
	}

	for _, ticket := range tickets {
		_, err := bunDB.NewInsert().Model(&ticket).Exec(context.Background())
		assert.NoError(t, err)
	}

	// Test case: Get total tickets count
	count, err := ticketCountDB.GetTotalTicketsCount()
	assert.NoError(t, err)
	assert.Equal(t, 3, count)
}