package booking

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{
		db: db,
	}
}

func (p *PostgresStore) MarkSeatBooked(
	ctx context.Context,
	showtimeID, seatID int64,
) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin booking transaction: %w", err)
	}
	defer tx.Rollback()

	var id int64

	err = tx.QueryRowContext(ctx, `
		UPDATE showtime_seats
		SET status = 'booked'
		WHERE showtime_id = $1
		  AND seat_id = $2
		  AND status = 'available'
		RETURNING id
	`, showtimeID, seatID).Scan(&id)

	if errors.Is(err, sql.ErrNoRows) {
		return ErrSeatUnavailable
	}

	if err != nil {
		return fmt.Errorf("update seat status: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit booking transaction: %w", err)
	}

	return nil
}

func (p *PostgresStore) HoldSeat(
	ctx context.Context,
	showtimeID, seatID int64,
) (SeatHold, error) {
	var hold SeatHold

	err := p.db.QueryRowContext(ctx, `
        UPDATE showtime_seats
        SET status = 'held',
            hold_id = gen_random_uuid(),
            hold_expires_at = NOW() + INTERVAL '2 minutes'
        WHERE showtime_id = $1
          AND seat_id = $2
          AND (
              status = 'available'
              OR (
                  status = 'held'
                  AND hold_expires_at <= NOW()
              )
          )
        RETURNING hold_id, hold_expires_at
    `, showtimeID, seatID).Scan(&hold.HoldID, &hold.ExpiresAt)

	if errors.Is(err, sql.ErrNoRows) {
		return SeatHold{}, ErrSeatUnavailable
	}

	if err != nil {
		return SeatHold{}, fmt.Errorf("hold seat: %w", err)
	}

	return hold, nil
}

func (p *PostgresStore) ConfirmSeat(
	ctx context.Context,
	showtimeID, seatID int64,
	holdID string,
) error {
	var id int64

	err := p.db.QueryRowContext(ctx, `
		UPDATE showtime_seats
		SET status = 'booked',
		    hold_id = NULL,
		    hold_expires_at = NULL
		WHERE showtime_id = $1
		  AND seat_id = $2
		  AND status = 'held'
		  AND hold_id = $3
		  AND hold_expires_at > NOW()
		RETURNING id
	`, showtimeID, seatID, holdID).Scan(&id)

	if errors.Is(err, sql.ErrNoRows) {
		return ErrSeatUnavailable
	}

	if err != nil {
		return fmt.Errorf("confirm seat: %w", err)
	}

	return nil
}

func (p *PostgresStore) ReleaseSeat(
	ctx context.Context,
	showtimeID, seatID int64,
	holdID string,
) error {
	var id int64

	err := p.db.QueryRowContext(ctx, `
        UPDATE showtime_seats
        SET status = 'available',
            hold_id = NULL,
            hold_expires_at = NULL
        WHERE showtime_id = $1
          AND seat_id = $2
          AND status = 'held'
          AND hold_id = $3
        RETURNING id
    `, showtimeID, seatID, holdID).Scan(&id)

	if errors.Is(err, sql.ErrNoRows) {
		return ErrSeatUnavailable
	}

	if err != nil {
		return fmt.Errorf("release seat: %w", err)
	}

	return nil
}
