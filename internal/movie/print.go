package movie

import "fmt"

func PrintMovies() {

	for id, movie := range movies {
		fmt.Printf("%d: %s\n", id, movie.Title)
	}
}
