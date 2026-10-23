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
