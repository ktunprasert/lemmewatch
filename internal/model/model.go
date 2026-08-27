package model

type MediaType string

const (
	Movie  MediaType = "movie"
	Series MediaType = "series"
)

type Media struct {
	ID      string
	Type    MediaType
	Name    string
	Year    int
	Poster  string
	Summary string
}

type Stream struct {
	Hash      string
	FileIndex int
	Title     string
	Filename  string
	Quality   int
	Seeders   int
	Size      int64
	Cached    bool
	Source    string
}
