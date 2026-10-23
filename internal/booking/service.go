package booking

import (
	"context"
	"errors"
)

var (
	ErrSeatUnavailable = errors.New("seat is unavailable")
	ErrInvalidBooking  = errors.New("invalid booking request")
)

type Store interface {
	MarkSeatBooked(ctx context.Context, showtimeID, seatID int64) error
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




