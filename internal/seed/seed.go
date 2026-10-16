package seed

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type movie struct {
	ID              int
	DurationMinutes int
}

type screen struct {
	ID int
}

type showtime struct {
	MovieID  int
	ScreenID int
	StartsAt time.Time
	EndsAt   time.Time
}

func Run(db *sql.DB) error {
	ctx := context.Background()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning seed transaction: %w", err)
	}
	defer tx.Rollback()

	if err := seedScreens(ctx, tx); err != nil {
		return err
	}

	movies, err := getMovies(ctx, tx)
	if err != nil {
		return err
	}
	if len(movies) == 0 {
		return fmt.Errorf("no movies found in database")
	}

	screens, err := getScreens(ctx, tx)
	if err != nil {
		return err
	}
	if len(screens) == 0 {
		return fmt.Errorf("no screens found in database")
	}

	if err := seedSeats(ctx, tx, screens); err != nil {
		return err
	}

	schedule := buildSchedule(movies, screens, time.Now(), 30)

	for _, show := range schedule {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO showtimes (
				movie_id,
				screen_id,
				starts_at,
				ends_at
			)
			SELECT $1, $2, $3, $4
			WHERE NOT EXISTS (
				SELECT 1
				FROM showtimes
				WHERE screen_id = $2
				  AND starts_at = $3
			)
		`,
			show.MovieID,
			show.ScreenID,
			show.StartsAt,
			show.EndsAt,
		)
		if err != nil {
			return fmt.Errorf(
				"inserting showtime for screen %d: %w",
				show.ScreenID,
				err,
			)
		}
	}

	if err := seedShowtimeSeats(ctx, tx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing seed transaction: %w", err)
	}

	return nil
}

func seedScreens(ctx context.Context, tx *sql.Tx) error {
	names := []string{
		"Screen 1",
		"Screen 2",
		"Screen 3",
	}

	for _, name := range names {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO screens (name)
			SELECT $1
			WHERE NOT EXISTS (
				SELECT 1
				FROM screens
				WHERE name = $1
			)
		`, name)
		if err != nil {
			return fmt.Errorf("seeding screen %q: %w", name, err)
		}
	}

	return nil
}

func getMovies(ctx context.Context, tx *sql.Tx) ([]movie, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, duration_minutes
		FROM movies
		ORDER BY id
		LIMIT 3
	`)
	if err != nil {
		return nil, fmt.Errorf("querying movies: %w", err)
	}
	defer rows.Close()

	var movies []movie

	for rows.Next() {
		var m movie
		if err := rows.Scan(&m.ID, &m.DurationMinutes); err != nil {
			return nil, fmt.Errorf("scanning movie: %w", err)
		}
		movies = append(movies, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating movies: %w", err)
	}

	return movies, nil
}

func getScreens(ctx context.Context, tx *sql.Tx) ([]screen, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id
		FROM screens
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("querying screens: %w", err)
	}
	defer rows.Close()

	var screens []screen

	for rows.Next() {
		var s screen
		if err := rows.Scan(&s.ID); err != nil {
			return nil, fmt.Errorf("scanning screen: %w", err)
		}
		screens = append(screens, s)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating screens: %w", err)
	}

	return screens, nil
}

func buildSchedule(
	movies []movie,
	screens []screen,
	now time.Time,
	days int,
) []showtime {
	if len(movies) == 0 || len(screens) == 0 || days <= 0 {
		return nil
	}

	const buffer = 20 * time.Minute

	startHours := []int{10, 14, 18}
	location := now.Location()

	today := time.Date(
		now.Year(), now.Month(), now.Day(),
		0, 0, 0, 0, location,
	)

	var schedule []showtime

	for day := 0; day < days; day++ {
		date := today.AddDate(0, 0, day)

		for screenIndex, s := range screens {
			for slotIndex, hour := range startHours {
				start := time.Date(
					date.Year(), date.Month(), date.Day(),
					hour, 0, 0, 0, location,
				)

				movieIndex := (day + screenIndex + slotIndex) % len(movies)
				m := movies[movieIndex]

				end := start.Add(
					time.Duration(m.DurationMinutes) * time.Minute,
				)

				// Leave enough time to clean before the next screening.
				if slotIndex+1 < len(startHours) {
					nextStart := time.Date(
						date.Year(), date.Month(), date.Day(),
						startHours[slotIndex+1], 0, 0, 0, location,
					)

					if end.Add(buffer).After(nextStart) {
						continue
					}
				}

				schedule = append(schedule, showtime{
					MovieID:  m.ID,
					ScreenID: s.ID,
					StartsAt: start,
					EndsAt:   end,
				})
			}
		}
	}

	return schedule
}

func seedSeats(ctx context.Context, tx *sql.Tx, screens []screen) error {
	for _, s := range screens {
		for row := 0; row < 5; row++ {
			for col := 1; col <= 10; col++ {
				seatNumber := fmt.Sprintf("%c%d", 'A'+row, col)

				_, err := tx.ExecContext(ctx, `
					INSERT INTO seats (screen_id, seat_number)
					VALUES ($1, $2)
					ON CONFLICT (screen_id, seat_number) DO NOTHING
				`, s.ID, seatNumber)
				if err != nil {
					return fmt.Errorf(
						"seeding seat %s for screen %d: %w",
						seatNumber, s.ID, err,
					)
				}
			}
		}
	}

	return nil
}
func seedShowtimeSeats(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO showtime_seats (showtime_id, seat_id, status)
		SELECT
			st.id,
			s.id,
			'available'
		FROM showtimes st
		JOIN seats s ON s.screen_id = st.screen_id
		ON CONFLICT (showtime_id, seat_id) DO NOTHING
	`)
	if err != nil {
		return fmt.Errorf("seeding showtime seats: %w", err)
	}

	return nil
}
