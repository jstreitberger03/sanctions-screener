package server_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/jstreitberger03/sanctions-screener/internal/server"
)

func setupTestServer(t *testing.T) (*server.Server, string) {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "test-sanctions-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	srv, err := server.New(server.Config{
		Port:   0,
		DBPath: tmpFile.Name(),
	})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	return srv, tmpFile.Name()
}

func TestHealthEndpoint(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestScreenEndpoint(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := map[string]interface{}{
		"name":      "John Smith",
		"threshold": 0.8,
		"lists":     []string{"OFAC"},
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/screen", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestScreenEndpointMissingName(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := map[string]interface{}{
		"threshold": 0.8,
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/screen", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestListsEndpoint(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/lists", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestBatchEndpoint(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := map[string]interface{}{
		"names":     []string{"John Smith", "Jane Doe"},
		"threshold": 0.8,
		"lists":     []string{"OFAC"},
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/screen/batch", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBatchEndpointEmptyNames(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := map[string]interface{}{
		"names":     []string{},
		"threshold": 0.8,
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/screen/batch", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestListCountEndpoint(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/lists/OFAC/count", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestGracefulShutdown(t *testing.T) {
	srv, dbPath := setupTestServer(t)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	// Give the server time to start listening.
	time.Sleep(50 * time.Millisecond)

	// Send SIGINT to trigger the graceful-shutdown path.
	_ = syscall.Kill(syscall.Getpid(), syscall.SIGINT) //nolint:errcheck // test: signal delivery for graceful shutdown

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("shutdown returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown timed out")
	}

	// Verify the store was properly closed: the DB file should be
	// readable (not locked) after ListenAndServe returns.
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM sanctions_list").Scan(&count); err != nil {
		t.Fatalf("query after shutdown: %v", err)
	}
}

func TestCORSHeaders(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("OPTIONS", "/api/v1/screen", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("OPTIONS preflight: expected 200, got %d", w.Code)
	}

	allowOrigin := w.Header().Get("Access-Control-Allow-Origin")
	if allowOrigin != "*" {
		t.Errorf("Access-Control-Allow-Origin: expected '*', got %q", allowOrigin)
	}

	allowMethods := w.Header().Get("Access-Control-Allow-Methods")
	if !strings.Contains(allowMethods, "POST") {
		t.Errorf("Access-Control-Allow-Methods should contain POST, got %q", allowMethods)
	}
}

func TestBatchEndpointWithMultipleNames(t *testing.T) {
	srv, _ := setupTestServer(t)

	names := make([]string, 20)
	for i := range 20 {
		names[i] = "Test Name"
	}
	body := map[string]interface{}{
		"names":     names,
		"threshold": 0.8,
		"lists":     []string{"OFAC"},
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/screen/batch", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Results      []map[string]interface{} `json:"results"`
		TotalMatches int                      `json:"total_matches"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if len(resp.Results) != len(names) {
		t.Errorf("expected %d results, got %d", len(names), len(resp.Results))
	}
}
