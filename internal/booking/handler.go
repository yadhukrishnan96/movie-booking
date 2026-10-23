

package booking

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/yadhukrishnan96/movie-booking/internal/utils"
)

type Booker interface {
	BookSeat(ctx context.Context, showtimeID, seatID int64) error
}

type Handler struct {
	service Booker
}

func NewHandler(service Booker) *Handler {
	return &Handler{
		service: service,
	}
}

func (h *Handler) BookSeat(w http.ResponseWriter, r *http.Request) {
	showtimeID, err := strconv.ParseInt(
		chi.URLParam(r, "showtimeID"), 10, 64,
	)
	if err != nil || showtimeID <= 0 {
		http.Error(w, "invalid showtime ID", http.StatusBadRequest)
		return
	}

	seatID, err := strconv.ParseInt(
		chi.URLParam(r, "seatID"), 10, 64,
	)
	if err != nil || seatID <= 0 {
		http.Error(w, "invalid seat ID", http.StatusBadRequest)
		return
	}

	err = h.service.BookSeat(r.Context(), showtimeID, seatID)
	if err != nil {
		if errors.Is(err, ErrInvalidBooking) {
			http.Error(w, "invalid booking request", http.StatusBadRequest)
			return
		}

		if errors.Is(err, ErrSeatUnavailable) {
			http.Error(w, "seat is unavailable", http.StatusConflict)
			return
		}

		http.Error(w, "failed to book seat", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "seat booked successfully",
	})
}