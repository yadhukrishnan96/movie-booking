package booking

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
	"github.com/yadhukrishnan96/movie-booking/internal/db"
)

type seatIDsRequest struct {
	SeatIDs []int64 `json:"seat_ids"`
}

type seatActionRequest struct {
	SeatIDs []int64 `json:"seat_ids"`
	HoldID  string  `json:"hold_id"`
}

// postJSON sends a JSON POST request to the supplied URL.
func postJSON(url string, payload any) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return http.Post(url, "application/json", bytes.NewReader(body))
}

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

	// Find an available seat to book.
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

			// Release all goroutines at approximately the same time.
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
		"%s/showtimes/%d/seats/hold",
		server.URL,
		showtimeID,
	)

	payload := seatIDsRequest{
		SeatIDs: []int64{seatID},
	}

	// First request should successfully hold the seat.
	resp, err := postJSON(url, payload)
	if err != nil {
		t.Fatalf("send first hold request: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf(
			"first request: expected 201 Created, got %d",
			resp.StatusCode,
		)
	}

	// The same seat cannot be held again while its hold is active.
	resp, err = postJSON(url, payload)
	if err != nil {
		t.Fatalf("send second hold request: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf(
			"second request: expected 409 Conflict, got %d",
			resp.StatusCode,
		)
	}
}

func TestConcurrentHold(t *testing.T) {
	database := openTestDB(t)

	showtimeID, seatID := findAvailableSeat(t, database)
	server := newBookingTestServer(t, database)

	url := fmt.Sprintf(
		"%s/showtimes/%d/seats/hold",
		server.URL,
		showtimeID,
	)

	const requests = 100

	payload, err := json.Marshal(seatIDsRequest{
		SeatIDs: []int64{seatID},
	})
	if err != nil {
		t.Fatalf("marshal hold request: %v", err)
	}

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

			resp, err := http.Post(
				url,
				"application/json",
				bytes.NewReader(payload),
			)
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
		successes.Load(),
		conflicts.Load(),
		unexpected.Load(),
	)

	if successes.Load() != 1 {
		t.Errorf("expected 1 successful hold, got %d", successes.Load())
	}

	if conflicts.Load() != requests-1 {
		t.Errorf(
			"expected %d conflicts, got %d",
			requests-1,
			conflicts.Load(),
		)
	}

	if unexpected.Load() != 0 {
		t.Errorf(
			"expected 0 unexpected responses, got %d",
			unexpected.Load(),
		)
	}
}

func TestExpiredHoldCanBeReclaimed(t *testing.T) {
	database := openTestDB(t)

	showtimeID, seatID := findAvailableSeat(t, database)
	server := newBookingTestServer(t, database)

	url := fmt.Sprintf(
		"%s/showtimes/%d/seats/hold",
		server.URL,
		showtimeID,
	)

	payload := seatIDsRequest{
		SeatIDs: []int64{seatID},
	}

	// Create the initial hold.
	resp, err := postJSON(url, payload)
	if err != nil {
		t.Fatalf("create initial hold: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf(
			"expected 201 for initial hold, got %d",
			resp.StatusCode,
		)
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
	resp, err = postJSON(url, payload)
	if err != nil {
		t.Fatalf("reclaim expired hold: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf(
			"expected 201 after expiry, got %d",
			resp.StatusCode,
		)
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
		_ = database.Close()
	})

	return database
}

func findAvailableSeat(
	t *testing.T,
	database *sql.DB,
) (int64, int64) {
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

func newBookingTestServer(
	t *testing.T,
	database *sql.DB,
) *httptest.Server {
	t.Helper()

	store := NewPostgresStore(database)
	service := NewService(store)
	handler := NewHandler(service)

	router := chi.NewRouter()

	router.Post(
		"/showtimes/{showtimeID}/seats/hold",
		handler.HoldSeat,
	)

	router.Post(
		"/showtimes/{showtimeID}/seats/confirm",
		handler.ConfirmSeat,
	)

	router.Post(
		"/showtimes/{showtimeID}/seats/release",
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
	holdResp, err := postJSON(
		fmt.Sprintf(
			"%s/showtimes/%d/seats/hold",
			server.URL,
			showtimeID,
		),
		seatIDsRequest{
			SeatIDs: []int64{seatID},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer holdResp.Body.Close()

	if holdResp.StatusCode != http.StatusCreated {
		t.Fatalf(
			"hold: expected %d, got %d",
			http.StatusCreated,
			holdResp.StatusCode,
		)
	}

	var hold SeatHold

	if err := json.NewDecoder(holdResp.Body).Decode(&hold); err != nil {
		t.Fatal(err)
	}

	// Confirm using the returned hold ID and seat ID.
	resp, err := postJSON(
		fmt.Sprintf(
			"%s/showtimes/%d/seats/confirm",
			server.URL,
			showtimeID,
		),
		seatActionRequest{
			SeatIDs: []int64{seatID},
			HoldID:  hold.HoldID,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf(
			"confirm: expected %d, got %d",
			http.StatusOK,
			resp.StatusCode,
		)
	}

	var status string

	err = database.QueryRow(
		`SELECT status
		 FROM showtime_seats
		 WHERE showtime_id = $1 AND seat_id = $2`,
		showtimeID,
		seatID,
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

	holdResp, err := postJSON(
		fmt.Sprintf(
			"%s/showtimes/%d/seats/hold",
			server.URL,
			showtimeID,
		),
		seatIDsRequest{
			SeatIDs: []int64{seatID},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer holdResp.Body.Close()

	if holdResp.StatusCode != http.StatusCreated {
		t.Fatalf(
			"hold: expected %d, got %d",
			http.StatusCreated,
			holdResp.StatusCode,
		)
	}

	resp, err := postJSON(
		fmt.Sprintf(
			"%s/showtimes/%d/seats/confirm",
			server.URL,
			showtimeID,
		),
		seatActionRequest{
			SeatIDs: []int64{seatID},
			HoldID:  "00000000-0000-0000-0000-000000000000",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf(
			"wrong hold ID: expected %d, got %d",
			http.StatusConflict,
			resp.StatusCode,
		)
	}
}

func TestConfirmSeatRejectsExpiredHold(t *testing.T) {
	database := openTestDB(t)

	showtimeID, seatID := findAvailableSeat(t, database)
	server := newBookingTestServer(t, database)

	holdResp, err := postJSON(
		fmt.Sprintf(
			"%s/showtimes/%d/seats/hold",
			server.URL,
			showtimeID,
		),
		seatIDsRequest{
			SeatIDs: []int64{seatID},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer holdResp.Body.Close()

	if holdResp.StatusCode != http.StatusCreated {
		t.Fatalf(
			"hold: expected %d, got %d",
			http.StatusCreated,
			holdResp.StatusCode,
		)
	}

	var hold SeatHold

	if err := json.NewDecoder(holdResp.Body).Decode(&hold); err != nil {
		t.Fatal(err)
	}

	// Force expiration so the test doesn't wait two minutes.
	_, err = database.Exec(
		`UPDATE showtime_seats
		 SET hold_expires_at = NOW() - INTERVAL '1 second'
		 WHERE showtime_id = $1 AND seat_id = $2 AND hold_id = $3`,
		showtimeID,
		seatID,
		hold.HoldID,
	)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := postJSON(
		fmt.Sprintf(
			"%s/showtimes/%d/seats/confirm",
			server.URL,
			showtimeID,
		),
		seatActionRequest{
			SeatIDs: []int64{seatID},
			HoldID:  hold.HoldID,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf(
			"expired hold: expected %d, got %d",
			http.StatusConflict,
			resp.StatusCode,
		)
	}
}

func TestReleaseSeat(t *testing.T) {
	database := openTestDB(t)

	showtimeID, seatID := findAvailableSeat(t, database)
	server := newBookingTestServer(t, database)

	// Create a hold.
	holdResp, err := postJSON(
		fmt.Sprintf(
			"%s/showtimes/%d/seats/hold",
			server.URL,
			showtimeID,
		),
		seatIDsRequest{
			SeatIDs: []int64{seatID},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if holdResp.StatusCode != http.StatusCreated {
		holdResp.Body.Close()
		t.Fatalf(
			"hold: expected %d, got %d",
			http.StatusCreated,
			holdResp.StatusCode,
		)
	}

	var hold SeatHold

	if err := json.NewDecoder(holdResp.Body).Decode(&hold); err != nil {
		holdResp.Body.Close()
		t.Fatal(err)
	}
	holdResp.Body.Close()

	// Release the seat using the matching hold ID.
	resp, err := postJSON(
		fmt.Sprintf(
			"%s/showtimes/%d/seats/release",
			server.URL,
			showtimeID,
		),
		seatActionRequest{
			SeatIDs: []int64{seatID},
			HoldID:  hold.HoldID,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf(
			"release: expected %d, got %d",
			http.StatusOK,
			resp.StatusCode,
		)
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
		t.Fatalf(
			"expected hold fields to be NULL, got hold_id=%v expires_at=%v",
			holdID,
			expiresAt,
		)
	}
}

func TestReleaseSeatRejectsWrongHoldID(t *testing.T) {
	database := openTestDB(t)

	showtimeID, seatID := findAvailableSeat(t, database)
	server := newBookingTestServer(t, database)

	holdResp, err := postJSON(
		fmt.Sprintf(
			"%s/showtimes/%d/seats/hold",
			server.URL,
			showtimeID,
		),
		seatIDsRequest{
			SeatIDs: []int64{seatID},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if holdResp.StatusCode != http.StatusCreated {
		holdResp.Body.Close()
		t.Fatalf(
			"hold: expected %d, got %d",
			http.StatusCreated,
			holdResp.StatusCode,
		)
	}
	holdResp.Body.Close()

	resp, err := postJSON(
		fmt.Sprintf(
			"%s/showtimes/%d/seats/release",
			server.URL,
			showtimeID,
		),
		seatActionRequest{
			SeatIDs: []int64{seatID},
			HoldID:  "00000000-0000-0000-0000-000000000000",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf(
			"wrong hold ID: expected %d, got %d",
			http.StatusConflict,
			resp.StatusCode,
		)
	}

	var status string

	err = database.QueryRow(`
		SELECT status
		FROM showtime_seats
		WHERE showtime_id = $1 AND seat_id = $2
	`, showtimeID, seatID).Scan(&status)
	if err != nil {
		t.Fatal(err)
	}

	if status != "held" {
		t.Fatalf(
			"wrong hold ID changed seat status: expected held, got %q",
			status,
		)
	}
}