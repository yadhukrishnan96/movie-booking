package booking

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/lib/pq"
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

func (p *PostgresStore) HoldSeats(
	ctx context.Context,
	showtimeID int64,
	seatIDs []int64,
) (SeatHold, error) {
	if showtimeID <= 0 || len(seatIDs) == 0 {
		return SeatHold{}, ErrInvalidBooking
	}

	ids := append([]int64(nil), seatIDs...)
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return SeatHold{}, fmt.Errorf("begin hold transaction: %w", err)
	}
	defer tx.Rollback()

	var hold SeatHold
	err = tx.QueryRowContext(ctx, `
        SELECT gen_random_uuid()::text, NOW() + INTERVAL '2 minutes'
    `).Scan(&hold.HoldID, &hold.ExpiresAt)
	if err != nil {
		return SeatHold{}, fmt.Errorf("create seat hold: %w", err)
	}

	for _, seatID := range ids {
		var id int64
		err = tx.QueryRowContext(ctx, `
            UPDATE showtime_seats
            SET status = 'held',
                hold_id = $3::uuid,
                hold_expires_at = $4
            WHERE showtime_id = $1
              AND seat_id = $2
              AND (
                  status = 'available'
                  OR (status = 'held' AND hold_expires_at <= NOW())
              )
            RETURNING id
        `, showtimeID, seatID, hold.HoldID, hold.ExpiresAt).Scan(&id)

		if errors.Is(err, sql.ErrNoRows) {
			return SeatHold{}, ErrSeatUnavailable
		}
		if err != nil {
			return SeatHold{}, fmt.Errorf("hold seat %d: %w", seatID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return SeatHold{}, fmt.Errorf("commit hold transaction: %w", err)
	}

	hold.SeatIDs = ids
	return hold, nil
}

func (p *PostgresStore) ConfirmSeats(
	ctx context.Context,
	showtimeID int64,
	seatIDs []int64,
	holdID string,
) error {

	if showtimeID <= 0 || len(seatIDs) == 0 || holdID == "" {
		return ErrInvalidBooking
	}

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin confirmation transaction: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
        UPDATE showtime_seats
        SET status = 'booked',
            hold_id = NULL,
            hold_expires_at = NULL
        WHERE showtime_id = $1
          AND seat_id = ANY($2)
          AND status = 'held'
          AND hold_id = $3
          AND hold_expires_at > NOW()
    `, showtimeID, pq.Array(seatIDs), holdID)
	if err != nil {
		return fmt.Errorf("confirm seats: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count confirmed seats: %w", err)
	}
	if affected != int64(len(seatIDs)) {
		return ErrInvalidHold
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit confirmation transaction: %w", err)
	}

	return nil
}

func (p *PostgresStore) ReleaseSeats(
	ctx context.Context,
	showtimeID int64,
	seatIDs []int64,
	holdID string,
) error {

	if showtimeID <= 0 || len(seatIDs) == 0 || holdID == "" {
		return ErrInvalidBooking
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin release transaction: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
        UPDATE showtime_seats
        SET status = 'available',
            hold_id = NULL,
            hold_expires_at = NULL
        WHERE showtime_id = $1
          AND seat_id = ANY($2)
          AND status = 'held'
          AND hold_id = $3
    `, showtimeID, pq.Array(seatIDs), holdID)
	if err != nil {
		return fmt.Errorf("release seats: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count released seats: %w", err)
	}
	if affected != int64(len(seatIDs)) {
		return ErrInvalidHold
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit release transaction: %w", err)
	}

	return nil
}
