
ALTER TABLE showtime_seats
    ADD COLUMN hold_id UUID,
    ADD COLUMN hold_expires_at TIMESTAMPTZ;

ALTER TABLE showtime_seats
    ADD CONSTRAINT showtime_seat_hold_fields_valid
    CHECK (
        (
            status = 'held'
            AND hold_id IS NOT NULL
            AND hold_expires_at IS NOT NULL
        )
        OR
        (
            status <> 'held'
            AND hold_id IS NULL
            AND hold_expires_at IS NULL
        )
    );
