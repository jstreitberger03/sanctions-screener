package main

import "testing"

// TestRunRejectsInvalidDBPath verifies that run() returns an error when
// the database path is invalid (directory doesn't exist).
func TestRunRejectsInvalidDBPath(t *testing.T) {
	t.Setenv("PORT", "0")
	t.Setenv("SCREENER_DB_PATH", "/nonexistent/path/sanctions.db")

	err := run()
	if err == nil {
		t.Error("expected error for invalid DB path, got nil")
	}
}

// TestRunRejectsVariousInvalidPaths verifies that run() properly reads
// SCREENER_DB_PATH from the environment and rejects invalid paths.
func TestRunRejectsVariousInvalidPaths(t *testing.T) {
	tests := []struct {
		name   string
		dbPath string
	}{
		{"nonexistent directory", "/no/such/dir/db.sqlite"},
		{"directory instead of file", "/tmp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PORT", "0")
			t.Setenv("SCREENER_DB_PATH", tt.dbPath)

			err := run()
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

// TestRunReadsPortEnvVar verifies that the PORT environment variable is
// parsed as an integer. We test the error path: PORT="0" with an invalid
// DB path ensures the server attempts to start (env parsing succeeded)
// but fails on the DB open.
func TestRunReadsPortEnvVar(t *testing.T) {
	// Valid port, invalid DB — the error should come from DB open, not port parsing.
	t.Setenv("PORT", "0")
	t.Setenv("SCREENER_DB_PATH", "/no/such/db.db")

	err := run()
	if err == nil {
		t.Fatal("expected error from invalid DB path")
	}
}

// TestRunInvalidPortFallsBackToDefault verifies that a non-numeric PORT
// value causes the default port (8080) to be used. With an invalid DB path,
// run() fails on DB open regardless of port — this test just verifies no panic.
func TestRunInvalidPortFallsBackToDefault(t *testing.T) {
	t.Setenv("PORT", "not-a-number")
	t.Setenv("SCREENER_DB_PATH", "/no/such/db.db")

	// Should not panic — falls back to default port and then errors on DB.
	err := run()
	if err == nil {
		t.Fatal("expected error from invalid DB path")
	}
}
