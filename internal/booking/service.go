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
	ExpiresAt time.Time `json:"expires_at"`
}
type Store interface {
	MarkSeatBooked(ctx context.Context, showtimeID, seatID int64) error
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

func (s *Service) HoldSeat(
	ctx context.Context,
	showtimeID, seatID int64,
) (SeatHold, error) {
	if showtimeID <= 0 || seatID <= 0 {
		return SeatHold{}, ErrInvalidBooking
	}

	return s.store.HoldSeat(ctx, showtimeID, seatID)
}

func (s *Service) ConfirmSeat(
	ctx context.Context,
	showtimeID, seatID int64,
	holdID string,
) error {
	if showtimeID <= 0 || seatID <= 0 || holdID == "" {
		return ErrInvalidBooking
	}

	return s.store.ConfirmSeat(ctx, showtimeID, seatID, holdID)
}

func (s *Service) ReleaseSeat(
	ctx context.Context,
	showtimeID, seatID int64,
	holdID string,
) error {
	if showtimeID <= 0 || seatID <= 0 || holdID == "" {
		return ErrInvalidBooking
	}

	return s.store.ReleaseSeat(ctx, showtimeID, seatID, holdID)
}
