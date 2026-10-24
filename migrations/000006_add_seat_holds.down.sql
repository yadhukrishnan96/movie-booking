
ALTER TABLE showtime_seats
    DROP CONSTRAINT showtime_seat_hold_fields_valid;

ALTER TABLE showtime_seats
    DROP COLUMN hold_expires_at,
    DROP COLUMN hold_id;
