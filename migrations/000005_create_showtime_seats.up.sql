
CREATE TABLE showtime_seats (
    id SERIAL PRIMARY KEY,
    showtime_id INTEGER NOT NULL REFERENCES showtimes(id),
    seat_id INTEGER NOT NULL REFERENCES seats(id),
    status TEXT NOT NULL DEFAULT 'available',

    CONSTRAINT showtime_seats_status_valid
        CHECK (status IN ('available', 'held', 'booked')),

    CONSTRAINT showtime_seat_unique
        UNIQUE (showtime_id, seat_id)
);
