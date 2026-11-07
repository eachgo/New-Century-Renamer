package main

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFile is a tiny helper for these tests.
func writeTestFile(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A folder dropped in as a whole tags every file it contributes with that
// folder's name; a loose file added on its own keeps an empty group.
func TestAddPathsGroupsByFolder(t *testing.T) {
	root := t.TempDir()
	album := filepath.Join(root, "Some Album")
	writeTestFile(t, filepath.Join(album, "01.mp3"), "a")
	writeTestFile(t, filepath.Join(album, "02.mp3"), "b")
	loose := filepath.Join(root, "loose.txt")
	writeTestFile(t, loose, "x")

	a := &app{stepSel: -1}
	a.addPaths([]string{album, loose})

	if len(a.files) != 3 {
		t.Fatalf("want 3 files, got %d", len(a.files))
	}
	grouped := 0
	looseCount := 0
	for _, f := range a.files {
		switch f.group {
		case "Some Album":
			grouped++
		case "":
			looseCount++
		default:
			t.Errorf("unexpected group %q for %s", f.group, f.path)
		}
	}
	if grouped != 2 {
		t.Errorf("want 2 files under 'Some Album', got %d", grouped)
	}
	if looseCount != 1 {
		t.Errorf("want 1 ungrouped file, got %d", looseCount)
	}
}

// Dropping a folder must not absorb a folder that was already added loose;
// and re-adding the same path (from either source) does not duplicate it.
func TestAddPathsDedupAcrossGroups(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Folder")
	writeTestFile(t, filepath.Join(dir, "a.txt"), "1")

	a := &app{stepSel: -1}
	a.addPaths([]string{filepath.Join(dir, "a.txt")}) // loose first
	a.addPaths([]string{dir})                         // then as a folder
	if len(a.files) != 1 {
		t.Fatalf("same file added twice by different paths: %d entries", len(a.files))
	}
}
