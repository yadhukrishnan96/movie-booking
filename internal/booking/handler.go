package booking

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/yadhukrishnan96/movie-booking/internal/utils"
)

type Booker interface {
	BookSeat(ctx context.Context, showtimeID, seatID int64) error
	HoldSeat(ctx context.Context, showtimeID, seatID int64) (SeatHold, error)

	ConfirmSeat(
		ctx context.Context,
		showtimeID, seatID int64,
		holdID string,
	) error

	ReleaseSeat(
		ctx context.Context,
		showtimeID, seatID int64,
		holdID string,
	) error
}

type confirmSeatRequest struct {
	HoldID string `json:"hold_id"`
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

func (h *Handler) HoldSeat(w http.ResponseWriter, r *http.Request) {
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

	hold, err := h.service.HoldSeat(r.Context(), showtimeID, seatID)
	if err != nil {
		if errors.Is(err, ErrInvalidBooking) {
			http.Error(w, "invalid booking request", http.StatusBadRequest)
			return
		}

		if errors.Is(err, ErrSeatUnavailable) {
			http.Error(w, "seat is unavailable", http.StatusConflict)
			return
		}

		http.Error(w, "failed to hold seat", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, http.StatusCreated, hold)
}

func (h *Handler) ConfirmSeat(w http.ResponseWriter, r *http.Request) {
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

	var request confirmSeatRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if request.HoldID == "" {
		http.Error(w, "hold_id is required", http.StatusBadRequest)
		return
	}

	err = h.service.ConfirmSeat(
		r.Context(),
		showtimeID,
		seatID,
		request.HoldID,
	)

	if errors.Is(err, ErrInvalidBooking) {
		http.Error(w, "invalid booking request", http.StatusBadRequest)
		return
	}

	if errors.Is(err, ErrSeatUnavailable) {
		http.Error(w, "seat hold is invalid, expired, or unavailable", http.StatusConflict)
		return
	}

	if err != nil {
		http.Error(w, "failed to confirm seat", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "seat booked successfully",
	})
}

func (h *Handler) ReleaseSeat(w http.ResponseWriter, r *http.Request) {
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

	var request confirmSeatRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if request.HoldID == "" {
		http.Error(w, "hold_id is required", http.StatusBadRequest)
		return
	}

	err = h.service.ReleaseSeat(
		r.Context(),
		showtimeID,
		seatID,
		request.HoldID,
	)

	if errors.Is(err, ErrInvalidBooking) {
		http.Error(w, "invalid booking request", http.StatusBadRequest)
		return
	}

	if errors.Is(err, ErrSeatUnavailable) {
		http.Error(w, "seat hold is invalid or unavailable", http.StatusConflict)
		return
	}

	if err != nil {
		http.Error(w, "failed to release seat", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "seat hold released successfully",
	})
}
