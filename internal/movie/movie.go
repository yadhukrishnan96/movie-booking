package movie

type Movie struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

var movies = map[int]Movie{
	1: {ID: 1, Title: "The Town"},
	2: {ID: 2, Title: "Sense and Sensibility"},
}


