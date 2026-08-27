package torbox

import "testing"

func TestVideoFilesPreserveAddonIndex(t *testing.T) {
	torrent := torrent{Files: []file{{ID: 1, Name: "poster.jpg"}, {ID: 2, Name: "movie.mkv"}, {ID: 3, Name: "sample.txt"}, {ID: 4, Name: "bonus.MP4"}}}
	files := torrent.videoFiles()
	if len(files) != 2 || files[0].ID != 2 || files[1].ID != 4 {
		t.Fatalf("files = %#v", files)
	}
}
