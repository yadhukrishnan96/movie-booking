package movie

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/yadhukrishnan96/movie-booking/internal/utils"
)

type Showtime struct {
	ID             int       `json:"id"`
	ScreenName     string    `json:"screen_name"`
	StartsAt       time.Time `json:"starts_at"`
	EndsAt         time.Time `json:"ends_at"`
	RemainingSeats int       `json:"remaining_seats"`
}

func (h *Handler) ListShowtimes(w http.ResponseWriter, r *http.Request) {
	movieID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || movieID <= 0 {
		http.Error(w, "invalid movie id", http.StatusBadRequest)
		return
	}

	rows, err := h.db.QueryContext(r.Context(), `
		SELECT
			st.id,
			s.name,
			st.starts_at,
			st.ends_at,
			COUNT(ss.id) FILTER (
				WHERE ss.status = 'available'
					OR (
						ss.status = 'held'
						AND ss.hold_expires_at <= NOW()
					)
			) AS remaining_seats
		FROM showtimes st
		JOIN screens s ON s.id = st.screen_id
		LEFT JOIN showtime_seats ss ON ss.showtime_id = st.id
		WHERE st.movie_id = $1
		GROUP BY st.id, s.name
		ORDER BY st.starts_at
	`, movieID)
	if err != nil {
		http.Error(w, "failed to fetch showtimes", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	showtimes := make([]Showtime, 0)

	for rows.Next() {
		var showtime Showtime

		if err := rows.Scan(
			&showtime.ID,
			&showtime.ScreenName,
			&showtime.StartsAt,
			&showtime.EndsAt,
			&showtime.RemainingSeats,
		); err != nil {
			fmt.Printf("ListShowtimes Scan error: %v\n", err)
			http.Error(w, "failed to read showtimes", http.StatusInternalServerError)
			return
		}

		showtimes = append(showtimes, showtime)
	}

	if err := rows.Err(); err != nil {
		fmt.Printf("ListShowtimes: iterating rows: %v\n", err)
		http.Error(w, "failed to read showtimes", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, http.StatusOK, showtimes)
}

type Seat struct {
	ID         int    `json:"seat_id"`
	SeatNumber string `json:"seat_number"`
	Status     string `json:"status"`
}

func (h *Handler) ListShowtimeSeats(w http.ResponseWriter, r *http.Request) {
	showtimeID, err := strconv.Atoi(chi.URLParam(r, "showtimeID"))
	if err != nil || showtimeID <= 0 {
		http.Error(w, "invalid showtime id", http.StatusBadRequest)
		return
	}

	// Confirm that the showtime exists, so an invalid ID returns 404
	// instead of an empty seat list.
	var exists bool
	err = h.db.QueryRowContext(
		r.Context(),
		"SELECT EXISTS (SELECT 1 FROM showtimes WHERE id = $1)",
		showtimeID,
	).Scan(&exists)
	if err != nil {
		http.Error(w, "failed to check showtime", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.NotFound(w, r)
		return
	}

	rows, err := h.db.QueryContext(r.Context(), `
		SELECT
			s.id,
			s.seat_number,
			CASE
				WHEN ss.id IS NULL THEN 'available'
				WHEN ss.status = 'held'
				     AND ss.hold_expires_at <= NOW()
				THEN 'available'
				ELSE ss.status
			END AS status
		FROM showtimes st
		JOIN seats s ON s.screen_id = st.screen_id
		LEFT JOIN showtime_seats ss
			ON ss.showtime_id = st.id
			AND ss.seat_id = s.id
		WHERE st.id = $1
		ORDER BY s.id
	`, showtimeID)
	if err != nil {
		http.Error(w, "failed to fetch seats", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	seats := make([]Seat, 0)

	for rows.Next() {
		var seat Seat

		if err := rows.Scan(
			&seat.ID,
			&seat.SeatNumber,
			&seat.Status,
		); err != nil {
			http.Error(w, "failed to read seats", http.StatusInternalServerError)
			return
		}

		seats = append(seats, seat)
	}

	if err := rows.Err(); err != nil {
		http.Error(w, "failed to read seats", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, http.StatusOK, seats)
}
