package booking

import (
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
