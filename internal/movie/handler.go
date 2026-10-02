package movie

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/yadhukrishnan96/movie-booking/internal/utils"
)

type Handler struct {
	db *sql.DB
}

func NewHandler(db *sql.DB) *Handler {
	return &Handler{
		db: db,
	}
}

func (h *Handler) UpdateMovie(w http.ResponseWriter, r *http.Request) {
	fmt.Println("UpdateMovie called")
	id, err := strconv.Atoi(chi.URLParam(r, "id"))

	if err != nil {

		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	var movie Movie

	err = json.NewDecoder(r.Body).Decode(&movie)

	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if movie.Title == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}


result, err := h.db.Exec(
		"UPDATE movies SET title = $1 WHERE id = $2",
		movie.Title,
		id,
	)
	if err != nil {
		http.Error(w, "failed to update movie", http.StatusInternalServerError)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		http.Error(w, "failed to check updated movie", http.StatusInternalServerError)
		return
	}

	if rowsAffected == 0 {
		http.NotFound(w, r)
		return
	}

	movie.ID = id

	

	utils.WriteJSON(w, http.StatusOK, movie)
}

func (h *Handler) DeleteMovie(w http.ResponseWriter, r *http.Request) {
	fmt.Println("DeleteMovie called")
	id, err := strconv.Atoi(chi.URLParam(r, "id"))

	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	result, err := h.db.Exec(
		"DELETE FROM movies WHERE id = $1", id,
	)

	if err != nil {
		http.Error(w, "failed to delete movie", http.StatusInternalServerError)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		http.Error(w, "failed to check deletion", http.StatusInternalServerError)
		return
	}

	if rowsAffected == 0 {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) CreateMovie(w http.ResponseWriter, r *http.Request) {
	var movie Movie

	err := json.NewDecoder(r.Body).Decode(&movie)

	if err != nil {
		http.Error(w, "invald request body", http.StatusBadRequest)
		return
	}
	if movie.Title == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return

	}

	h.db.QueryRow(
		"INSERT INTO movies (title) VALUES ($1) RETURNING id",
		movie.Title,
	).Scan(&movie.ID)

}

func (h *Handler) GetMovie(w http.ResponseWriter, r *http.Request) {

	fmt.Println("URL param:", chi.URLParam(r, "id"))
	id, err := strconv.Atoi(chi.URLParam(r, "id"))

	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	var movie Movie

	err = h.db.QueryRow(
		"SELECT id, title FROM movies WHERE id = $1", id,
	).Scan(&movie.ID, &movie.Title)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)

			return
		}

		http.Error(w, "failed to fetch movie", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, http.StatusOK, movie)

}

func (h *Handler) ListMovies(w http.ResponseWriter, r *http.Request) {

	rows, err := h.db.Query("SELECT id, title FROM movies")

	if err != nil {
		http.Error(w, "failed to fetch movies", http.StatusInternalServerError)
		return
	}

	defer rows.Close()

	var movies []Movie

	for rows.Next() {
		var movie Movie

		err := rows.Scan(&movie.ID, &movie.Title)

		if err != nil {

			http.Error(w, "failed to read movie", http.StatusInternalServerError)

			return
		}
		movies = append(movies, movie)
	}

	if err := rows.Err(); err != nil {

		http.Error(w, "failed to read movies", http.StatusInternalServerError)

	}
	utils.WriteJSON(w, http.StatusOK, movies)
}
