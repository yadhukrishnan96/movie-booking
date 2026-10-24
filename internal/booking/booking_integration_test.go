package booking

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
	"github.com/yadhukrishnan96/movie-booking/internal/db"
)

func TestConcurrentBooking(t *testing.T) {
	// Load the project's .env when running this test locally.
	_ = godotenv.Load("../../.env")

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL is not set; skipping integration test")
	}

	database, err := db.Connect()
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}
	defer database.Close()

	// Find a seat that can be booked.
	var showtimeID, seatID int64

	err = database.QueryRow(`
		SELECT showtime_id, seat_id
		FROM showtime_seats
		WHERE status = 'available'
		LIMIT 1
	`).Scan(&showtimeID, &seatID)
	if err != nil {
		t.Fatalf("find available seat: %v", err)
	}

	// Build the real handler -> service -> PostgreSQL store chain.
	store := NewPostgresStore(database)
	service := NewService(store)
	handler := NewHandler(service)

	router := chi.NewRouter()
	router.Post(
		"/showtimes/{showtimeID}/seats/{seatID}/book",
		handler.BookSeat,
	)

	server := httptest.NewServer(router)
	defer server.Close()

	const requests = 10000

	var successes atomic.Int64
	var conflicts atomic.Int64
	var unexpected atomic.Int64

	start := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < requests; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			// All goroutines wait here until released together.
			<-start

			url := fmt.Sprintf(
				"%s/showtimes/%d/seats/%d/book",
				server.URL,
				showtimeID,
				seatID,
			)

			resp, err := http.Post(url, "", nil)
			if err != nil {
				unexpected.Add(1)
				t.Errorf("send booking request: %v", err)
				return
			}
			defer resp.Body.Close()

			switch resp.StatusCode {
			case http.StatusOK:
				successes.Add(1)
			case http.StatusConflict:
				conflicts.Add(1)
			default:
				unexpected.Add(1)
				t.Errorf("unexpected HTTP status: %d", resp.StatusCode)
			}
		}()
	}

	// Release all 100 requests.
	close(start)
	wg.Wait()

	t.Logf(
		"Results: successes=%d conflicts=%d unexpected=%d",
		successes.Load(),
		conflicts.Load(),
		unexpected.Load(),
	)

	if got := successes.Load(); got != 1 {
		t.Errorf("expected exactly 1 successful booking, got %d", got)
	}

	if got := conflicts.Load(); got != requests-1 {
		t.Errorf("expected %d conflicts, got %d", requests-1, got)
	}

	if got := unexpected.Load(); got != 0 {
		t.Errorf("expected 0 unexpected responses, got %d", got)
	}

	// Verify the database's final state.
	var status string
	err = database.QueryRow(`
		SELECT status
		FROM showtime_seats
		WHERE showtime_id = $1 AND seat_id = $2
	`, showtimeID, seatID).Scan(&status)
	if err != nil {
		t.Fatalf("check final seat status: %v", err)
	}

	if status != "booked" {
		t.Errorf("expected final status booked, got %q", status)
	}
}

func TestHoldSeat(t *testing.T) {
	database := openTestDB(t)

	showtimeID, seatID := findAvailableSeat(t, database)
	server := newBookingTestServer(t, database)

	url := fmt.Sprintf(
		"%s/showtimes/%d/seats/%d/hold",
		server.URL, showtimeID, seatID,
	)

	// First request should create the hold.
	resp, err := http.Post(url, "", nil)
	if err != nil {
		t.Fatalf("send first hold request: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("first request: expected 201, got %d", resp.StatusCode)
	}

	// Second request must not replace the active hold.
	resp, err = http.Post(url, "", nil)
	if err != nil {
		t.Fatalf("send second hold request: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("second request: expected 409, got %d", resp.StatusCode)
	}
}

func TestConcurrentHold(t *testing.T) {
	database := openTestDB(t)

	showtimeID, seatID := findAvailableSeat(t, database)
	server := newBookingTestServer(t, database)

	const requests = 100

	var successes atomic.Int64
	var conflicts atomic.Int64
	var unexpected atomic.Int64

	start := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < requests; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()
			<-start

			url := fmt.Sprintf(
				"%s/showtimes/%d/seats/%d/hold",
				server.URL, showtimeID, seatID,
			)

			resp, err := http.Post(url, "", nil)
			if err != nil {
				unexpected.Add(1)
				t.Errorf("send hold request: %v", err)
				return
			}
			defer resp.Body.Close()

			switch resp.StatusCode {
			case http.StatusCreated:
				successes.Add(1)
			case http.StatusConflict:
				conflicts.Add(1)
			default:
				unexpected.Add(1)
				t.Errorf("unexpected status: %d", resp.StatusCode)
			}
		}()
	}

	close(start)
	wg.Wait()

	t.Logf(
		"Hold results: successes=%d conflicts=%d unexpected=%d",
		successes.Load(), conflicts.Load(), unexpected.Load(),
	)

	if successes.Load() != 1 {
		t.Errorf("expected 1 successful hold, got %d", successes.Load())
	}
	if conflicts.Load() != requests-1 {
		t.Errorf("expected %d conflicts, got %d", requests-1, conflicts.Load())
	}
	if unexpected.Load() != 0 {
		t.Errorf("expected 0 unexpected responses, got %d", unexpected.Load())
	}
}

func TestExpiredHoldCanBeReclaimed(t *testing.T) {
	database := openTestDB(t)

	showtimeID, seatID := findAvailableSeat(t, database)
	server := newBookingTestServer(t, database)

	// Create the initial hold.
	url := fmt.Sprintf(
		"%s/showtimes/%d/seats/%d/hold",
		server.URL, showtimeID, seatID,
	)

	resp, err := http.Post(url, "", nil)
	if err != nil {
		t.Fatalf("create initial hold: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for initial hold, got %d", resp.StatusCode)
	}

	var originalHoldID string

	err = database.QueryRow(`
		SELECT hold_id
		FROM showtime_seats
		WHERE showtime_id = $1 AND seat_id = $2
	`, showtimeID, seatID).Scan(&originalHoldID)
	if err != nil {
		t.Fatalf("read initial hold ID: %v", err)
	}

	// Expire the hold directly in the test database.
	_, err = database.Exec(`
		UPDATE showtime_seats
		SET hold_expires_at = NOW() - INTERVAL '1 second'
		WHERE showtime_id = $1 AND seat_id = $2
	`, showtimeID, seatID)
	if err != nil {
		t.Fatalf("expire hold: %v", err)
	}

	// A new request should reclaim the expired hold.
	resp, err = http.Post(url, "", nil)
	if err != nil {
		t.Fatalf("reclaim expired hold: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 after expiry, got %d", resp.StatusCode)
	}

	var newHoldID string

	err = database.QueryRow(`
		SELECT hold_id
		FROM showtime_seats
		WHERE showtime_id = $1 AND seat_id = $2
	`, showtimeID, seatID).Scan(&newHoldID)
	if err != nil {
		t.Fatalf("read new hold ID: %v", err)
	}

	if newHoldID == originalHoldID {
		t.Error("expected a new hold ID after expiry")
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	_ = godotenv.Load("../../.env")

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL is not set; skipping integration test")
	}

	database, err := db.Connect()
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}

	t.Cleanup(func() {
		database.Close()
	})

	return database
}

func findAvailableSeat(t *testing.T, database *sql.DB) (int64, int64) {
	t.Helper()

	var showtimeID, seatID int64

	err := database.QueryRow(`
		SELECT showtime_id, seat_id
		FROM showtime_seats
		WHERE status = 'available'
		LIMIT 1
	`).Scan(&showtimeID, &seatID)

	if err != nil {
		t.Fatalf("find available seat: %v", err)
	}

	return showtimeID, seatID
}

func newBookingTestServer(t *testing.T, database *sql.DB) *httptest.Server {
	t.Helper()

	store := NewPostgresStore(database)
	service := NewService(store)
	handler := NewHandler(service)

	router := chi.NewRouter()
	router.Post(
		"/showtimes/{showtimeID}/seats/{seatID}/hold",
		handler.HoldSeat,
	)

	router.Post(
		"/showtimes/{showtimeID}/seats/{seatID}/confirm",
		handler.ConfirmSeat,
	)

	router.Post(
		"/showtimes/{showtimeID}/seats/{seatID}/release",
		handler.ReleaseSeat,
	)

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	return server
}

func TestConfirmSeat(t *testing.T) {
	database := openTestDB(t)
	showtimeID, seatID := findAvailableSeat(t, database)
	server := newBookingTestServer(t, database)

	// Create a hold.
	holdResp, err := http.Post(
		fmt.Sprintf("%s/showtimes/%d/seats/%d/hold", server.URL, showtimeID, seatID),
		"", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer holdResp.Body.Close()

	if holdResp.StatusCode != http.StatusCreated {
		t.Fatalf("hold: expected %d, got %d", http.StatusCreated, holdResp.StatusCode)
	}

	var hold SeatHold
	if err := json.NewDecoder(holdResp.Body).Decode(&hold); err != nil {
		t.Fatal(err)
	}

	// Confirm using the returned hold ID.
	body := strings.NewReader(fmt.Sprintf(`{"hold_id":%q}`, hold.HoldID))
	req, err := http.NewRequest(
		http.MethodPost,
		fmt.Sprintf("%s/showtimes/%d/seats/%d/confirm", server.URL, showtimeID, seatID),
		body,
	)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("confirm: expected %d, got %d", http.StatusOK, resp.StatusCode)
	}

	var status string
	err = database.QueryRow(
		`SELECT status FROM showtime_seats WHERE showtime_id = $1 AND seat_id = $2`,
		showtimeID, seatID,
	).Scan(&status)
	if err != nil {
		t.Fatal(err)
	}
	if status != "booked" {
		t.Fatalf("expected booked, got %q", status)
	}
}

func TestConfirmSeatRejectsWrongHoldID(t *testing.T) {
	database := openTestDB(t)
	showtimeID, seatID := findAvailableSeat(t, database)
	server := newBookingTestServer(t, database)

	holdResp, err := http.Post(
		fmt.Sprintf("%s/showtimes/%d/seats/%d/hold", server.URL, showtimeID, seatID),
		"", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer holdResp.Body.Close()

	if holdResp.StatusCode != http.StatusCreated {
		t.Fatalf("hold: expected %d, got %d", http.StatusCreated, holdResp.StatusCode)
	}

	body := strings.NewReader(`{"hold_id":"00000000-0000-0000-0000-000000000000"}`)
	req, err := http.NewRequest(
		http.MethodPost,
		fmt.Sprintf("%s/showtimes/%d/seats/%d/confirm", server.URL, showtimeID, seatID),
		body,
	)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("wrong hold ID: expected %d, got %d", http.StatusConflict, resp.StatusCode)
	}
}

func TestConfirmSeatRejectsExpiredHold(t *testing.T) {
	database := openTestDB(t)
	showtimeID, seatID := findAvailableSeat(t, database)
	server := newBookingTestServer(t, database)

	holdResp, err := http.Post(
		fmt.Sprintf("%s/showtimes/%d/seats/%d/hold", server.URL, showtimeID, seatID),
		"", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer holdResp.Body.Close()

	if holdResp.StatusCode != http.StatusCreated {
		t.Fatalf("hold: expected %d, got %d", http.StatusCreated, holdResp.StatusCode)
	}

	var hold SeatHold
	if err := json.NewDecoder(holdResp.Body).Decode(&hold); err != nil {
		t.Fatal(err)
	}

	// Force expiration in the database so the test doesn't wait two minutes.
	_, err = database.Exec(
		`UPDATE showtime_seats
		 SET hold_expires_at = NOW() - INTERVAL '1 second'
		 WHERE showtime_id = $1 AND seat_id = $2 AND hold_id = $3`,
		showtimeID, seatID, hold.HoldID,
	)
	if err != nil {
		t.Fatal(err)
	}

	body := strings.NewReader(fmt.Sprintf(`{"hold_id":%q}`, hold.HoldID))
	req, err := http.NewRequest(
		http.MethodPost,
		fmt.Sprintf("%s/showtimes/%d/seats/%d/confirm", server.URL, showtimeID, seatID),
		body,
	)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expired hold: expected %d, got %d", http.StatusConflict, resp.StatusCode)
	}
}

func TestReleaseSeat(t *testing.T) {
	database := openTestDB(t)
	showtimeID, seatID := findAvailableSeat(t, database)
	server := newBookingTestServer(t, database)

	// Create a hold.
	holdResp, err := http.Post(
		fmt.Sprintf("%s/showtimes/%d/seats/%d/hold", server.URL, showtimeID, seatID),
		"", nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	var hold SeatHold
	if holdResp.StatusCode != http.StatusCreated {
		holdResp.Body.Close()
		t.Fatalf("hold: expected %d, got %d", http.StatusCreated, holdResp.StatusCode)
	}
	if err := json.NewDecoder(holdResp.Body).Decode(&hold); err != nil {
		holdResp.Body.Close()
		t.Fatal(err)
	}
	holdResp.Body.Close()

	// Release the seat using the matching hold ID.
	body := strings.NewReader(fmt.Sprintf(`{"hold_id":%q}`, hold.HoldID))
	req, err := http.NewRequest(
		http.MethodPost,
		fmt.Sprintf("%s/showtimes/%d/seats/%d/release", server.URL, showtimeID, seatID),
		body,
	)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("release: expected %d, got %d", http.StatusOK, resp.StatusCode)
	}

	var status string
	var holdID sql.NullString
	var expiresAt sql.NullTime
	err = database.QueryRow(`
		SELECT status, hold_id, hold_expires_at
		FROM showtime_seats
		WHERE showtime_id = $1 AND seat_id = $2
	`, showtimeID, seatID).Scan(&status, &holdID, &expiresAt)
	if err != nil {
		t.Fatal(err)
	}

	if status != "available" {
		t.Fatalf("expected available, got %q", status)
	}
	if holdID.Valid || expiresAt.Valid {
		t.Fatalf("expected hold fields to be NULL, got hold_id=%v expires_at=%v", holdID, expiresAt)
	}
}

func TestReleaseSeatRejectsWrongHoldID(t *testing.T) {
	database := openTestDB(t)
	showtimeID, seatID := findAvailableSeat(t, database)
	server := newBookingTestServer(t, database)

	holdResp, err := http.Post(
		fmt.Sprintf("%s/showtimes/%d/seats/%d/hold", server.URL, showtimeID, seatID),
		"", nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if holdResp.StatusCode != http.StatusCreated {
		holdResp.Body.Close()
		t.Fatalf("hold: expected %d, got %d", http.StatusCreated, holdResp.StatusCode)
	}
	holdResp.Body.Close()

	body := strings.NewReader(`{"hold_id":"00000000-0000-0000-0000-000000000000"}`)
	req, err := http.NewRequest(
		http.MethodPost,
		fmt.Sprintf("%s/showtimes/%d/seats/%d/release", server.URL, showtimeID, seatID),
		body,
	)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("wrong hold ID: expected %d, got %d", http.StatusConflict, resp.StatusCode)
	}

	var status string
	err = database.QueryRow(`
		SELECT status FROM showtime_seats
		WHERE showtime_id = $1 AND seat_id = $2
	`, showtimeID, seatID).Scan(&status)
	if err != nil {
		t.Fatal(err)
	}
	if status != "held" {
		t.Fatalf("wrong hold ID changed seat status: expected held, got %q", status)
	}
}
