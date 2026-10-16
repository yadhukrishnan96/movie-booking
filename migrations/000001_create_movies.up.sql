

CREATE TABLE movies (
    id SERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    duration_minutes INTEGER NOT NULL DEFAULT 120,

    CONSTRAINT movies_duration_positive
        CHECK (duration_minutes > 0)
);