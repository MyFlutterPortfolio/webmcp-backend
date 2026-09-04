package postgres

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMigrationsSortsForwardFilesAndSkipsDownFiles(t *testing.T) {
	directory := t.TempDir()
	files := map[string]string{
		"000002_second.up.sql":  "SELECT 2;",
		"000001_first.up.sql":   "SELECT 1;",
		"000001_first.down.sql": "SELECT broken;",
		"README.md":             "ignored",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	migrations, err := LoadMigrations(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 2 || migrations[0].Version != 1 || migrations[1].Version != 2 {
		t.Fatalf("unexpected migrations: %#v", migrations)
	}
}

func TestLoadMigrationsRejectsDuplicateVersions(t *testing.T) {
	directory := t.TempDir()
	for _, name := range []string{"000001_first.up.sql", "000001_second.up.sql"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("SELECT 1;"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := LoadMigrations(directory); err == nil {
		t.Fatal("expected duplicate version error")
	}
}
