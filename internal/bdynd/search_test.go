package bdynd

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSearchPacksFiltersByTypeNameAndCreatedTime(t *testing.T) {
	r := newTestRepo(t)
	writeFile(t, filepath.Join(r.Root, "logs/app-2026.txt"), "log\n")
	writeFile(t, filepath.Join(r.Root, "images/app.png"), "png\n")
	must(t, Add(r, []string{"logs", "images"}))
	if _, err := Commit(r, CommitOptions{Message: "dataset"}); err != nil {
		t.Fatal(err)
	}
	manifest, err := Pack(r, PackOptions{Name: "batch"})
	if err != nil {
		t.Fatal(err)
	}

	results, err := SearchPacks(r, SearchOptions{
		Type:  "txt",
		Name:  "app",
		Since: manifest.CreatedAt.Add(-time.Minute),
		Until: manifest.CreatedAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results=%+v", results)
	}
	if results[0].Path != "logs/app-2026.txt" || results[0].PackID != manifest.ID {
		t.Fatalf("result=%+v", results[0])
	}
}
