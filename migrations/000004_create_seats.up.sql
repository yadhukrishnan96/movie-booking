

CREATE TABLE seats (
    id SERIAL PRIMARY KEY,
    screen_id INTEGER NOT NULL REFERENCES screens(id),
    seat_number TEXT NOT NULL,

    CONSTRAINT seats_screen_seat_unique
        UNIQUE (screen_id, seat_number)
);