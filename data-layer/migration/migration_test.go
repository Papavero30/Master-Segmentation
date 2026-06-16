package migration

import (
	"strings"
	"testing"
)

func TestGetMigrationsIncludesSegmentationExperiments(t *testing.T) {
	migrations := GetMigrations()
	if len(migrations) == 0 {
		t.Fatal("expected non-empty migration list")
	}

	seen := map[string]struct{}{}
	var found bool

	for _, m := range migrations {
		if _, exists := seen[m.Name]; exists {
			t.Fatalf("duplicate migration name found: %s", m.Name)
		}
		seen[m.Name] = struct{}{}

		if m.Name == "007_add_segmentation_experiments" {
			found = true
			if !strings.Contains(m.UpSQL, "CREATE TABLE IF NOT EXISTS segmentation_experiments") {
				t.Fatal("migration 007 must create segmentation_experiments table")
			}
			if !strings.Contains(m.UpSQL, "CREATE TABLE IF NOT EXISTS segmentation_chunk_logs") {
				t.Fatal("migration 007 must create segmentation_chunk_logs table")
			}
			if !strings.Contains(m.DownSQL, "DROP TABLE IF EXISTS segmentation_chunk_logs") {
				t.Fatal("migration 007 down sql must drop segmentation_chunk_logs")
			}
			if !strings.Contains(m.DownSQL, "DROP TABLE IF EXISTS segmentation_experiments") {
				t.Fatal("migration 007 down sql must drop segmentation_experiments")
			}
		}
	}

	if !found {
		t.Fatal("expected migration 007_add_segmentation_experiments to be registered")
	}
}
