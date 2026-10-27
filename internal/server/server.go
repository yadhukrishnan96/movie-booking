package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/yadhukrishnan96/movie-booking/internal/booking"
	"github.com/yadhukrishnan96/movie-booking/internal/movie"
)

type Server struct {
	httpServer *http.Server
	router     chi.Router

	movieHandler   *movie.Handler
	bookingHandler *booking.Handler
}

func NewServer(movieHandler *movie.Handler, bookingHandler *booking.Handler) *Server {

	r := chi.NewRouter()

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"http://localhost:5173"},
		AllowedMethods: []string{
			"GET",
			"POST",
			"PUT",
			"DELETE",
			"OPTIONS",
		},
		AllowedHeaders: []string{
			"Accept",
			"Authorization",
			"Content-Type",
		},
		MaxAge: 300,
	}))

	return &Server{
		httpServer: &http.Server{
			Addr:    ":3000",
			Handler: r,
		},
		router:         r,
		movieHandler:   movieHandler,
		bookingHandler: bookingHandler,
	}
}

func (s *Server) Mount() {
	s.router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	s.router.Get("/movies/{id}", s.movieHandler.GetMovie)
	s.router.Get("/movies", s.movieHandler.ListMovies)
	s.router.Post("/movies", s.movieHandler.CreateMovie)
	s.router.Delete("/movies/{id}", s.movieHandler.DeleteMovie)
	s.router.Put("/movies/{id}", s.movieHandler.UpdateMovie)

	s.router.Post(
		"/showtimes/{showtimeID}/seats/{seatID}/book",
		s.bookingHandler.BookSeat,
	)

	s.router.Post(
		"/showtimes/{showtimeID}/seats/{seatID}/hold",
		s.bookingHandler.HoldSeat,
	)

	s.router.Post(
		"/showtimes/{showtimeID}/seats/{seatID}/confirm",
		s.bookingHandler.ConfirmSeat,
	)

	s.router.Post(
		"/showtimes/{showtimeID}/seats/{seatID}/release",
		s.bookingHandler.ReleaseSeat,
	)

	s.router.Get("/movies/{id}/showtimes", s.movieHandler.ListShowtimes)

	s.router.Get(
		"/showtimes/{showtimeID}/seats",
		s.movieHandler.ListShowtimeSeats,
	)
}

func (s *Server) Run() error {
	return s.httpServer.ListenAndServe()
}
