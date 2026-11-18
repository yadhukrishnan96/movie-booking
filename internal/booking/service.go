package booking

import (
	"context"
	"errors"
	"time"
)

var (
	ErrSeatUnavailable = errors.New("seat is unavailable")
	ErrInvalidBooking  = errors.New("invalid booking request")
	ErrInvalidHold     = errors.New("invalid or expired seat hold")
)

type SeatHold struct {
	HoldID    string    `json:"hold_id"`
	SeatIDs   []int64   `json:"seat_ids"`
	ExpiresAt time.Time `json:"expires_at"`
}
type Store interface {
	MarkSeatBooked(ctx context.Context, showtimeID, seatID int64) error
	HoldSeats(ctx context.Context, showtimeID int64, seatIDs []int64) (SeatHold, error)

	ConfirmSeats(
		ctx context.Context,
		showtimeID int64, seatIDs []int64,
		holdID string,
	) error

	ReleaseSeats(
		ctx context.Context,
		showtimeID int64, seatIDs []int64,
		holdID string,
	) error
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{
		store: store,
	}
}

func (s *Service) BookSeat(
	ctx context.Context,
	showtimeID, seatID int64,
) error {
	if showtimeID <= 0 || seatID <= 0 {
		return ErrInvalidBooking
	}

	return s.store.MarkSeatBooked(ctx, showtimeID, seatID)
}

func (s *Service) HoldSeats(
	ctx context.Context,
	showtimeID int64,
	seatIDs []int64,
) (SeatHold, error) {
	if showtimeID <= 0 || len(seatIDs) == 0 {
		return SeatHold{}, ErrInvalidBooking
	}

	seen := make(map[int64]struct{}, len(seatIDs))
	for _, seatID := range seatIDs {
		if seatID <= 0 {
			return SeatHold{}, ErrInvalidBooking
		}
		if _, exists := seen[seatID]; exists {
			return SeatHold{}, ErrInvalidBooking
		}
		seen[seatID] = struct{}{}
	}

	return s.store.HoldSeats(ctx, showtimeID, seatIDs)
}

func (s *Service) ConfirmSeats(
	ctx context.Context,
	showtimeID int64,
	seatIDs []int64,
	holdID string,
) error {
	if showtimeID <= 0 || len(seatIDs) == 0 || holdID == "" {
		return ErrInvalidBooking
	}

	return s.store.ConfirmSeats(ctx, showtimeID, seatIDs, holdID)
}

func (s *Service) ReleaseSeats(
	ctx context.Context,
	showtimeID int64,
	seatIDs []int64,
	holdID string,
) error {
	if showtimeID <= 0 || len(seatIDs) == 0 || holdID == "" {
		return ErrInvalidBooking
	}

	return s.store.ReleaseSeats(ctx, showtimeID, seatIDs, holdID)
}
