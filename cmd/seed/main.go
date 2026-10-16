package main

import (
	"fmt"
	"log"

	"github.com/joho/godotenv"
	"github.com/yadhukrishnan96/movie-booking/internal/db"
	"github.com/yadhukrishnan96/movie-booking/internal/seed"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if err := godotenv.Load(); err != nil {
		return fmt.Errorf("loading env: %w", err)
	}

	database, err := db.Connect()
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer database.Close()

	if err := seed.Run(database); err != nil {
		return fmt.Errorf("running seed: %w", err)
	}

	fmt.Println("Seeding completed.")
	return nil
}
