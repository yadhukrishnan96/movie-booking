package main

import (
	"fmt"
	"log"

	"github.com/joho/godotenv"
	"github.com/yadhukrishnan96/movie-booking/internal/db"
	"github.com/yadhukrishnan96/movie-booking/internal/movie"
	"github.com/yadhukrishnan96/movie-booking/internal/server"
)

func main() {
	if err := run(); err != nil {

		log.Fatal(err)

	}
	println("server initiated")
}

func run() error {

	// loading .env confs

	if err := godotenv.Load(); err != nil {
		return fmt.Errorf("loading env: %w", err)
	}

	//  postgress

	database, err := db.Connect()

	if err != nil {

		return fmt.Errorf("connecting to database: %w", err)

	}
	defer database.Close()
	
	movieHandler := movie.NewHandler(database)


	// initiating the server
	srv := server.NewServer(movieHandler)

	srv.Mount()

	return srv.Run()

}
