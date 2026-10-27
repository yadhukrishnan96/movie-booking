package movie

type Movie struct {
	ID              int    `json:"id"`
	Title           string `json:"title"`
	DurationMinutes int    `json:"duration_minutes"`
}
