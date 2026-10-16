

CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE showtimes (
    id SERIAL PRIMARY KEY,

    movie_id INTEGER NOT NULL
        REFERENCES movies(id),

    screen_id INTEGER NOT NULL
        REFERENCES screens(id),

    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT showtimes_valid_times
        CHECK (ends_at > starts_at),

    CONSTRAINT showtimes_no_overlap
        EXCLUDE USING gist (
            screen_id WITH =,
            tstzrange(starts_at, ends_at, '[)') WITH &&
        )
);