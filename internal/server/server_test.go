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
	"github.com/jstreitberger03/sanctions-screener/pkg/models"
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

// TestScreenExplicitThresholdZero verifies that threshold=0 is honored as a
// valid value, not silently replaced with 0.8. With threshold=0, a low-score
// fuzzy match (below 0.8) should still be returned. If the server replaces 0
// with 0.8, this match would be filtered out.
// TestScreenAllListsFail500 verifies that when the store is broken (closed),
// POST /screen returns 500 instead of 200 with 0 matches.
// TestListsEndpointReturnsArrayNotNull verifies that GET /lists always returns
// a JSON array (possibly empty), never null. When all lists fail to load
// (broken store), the response must still be [] not null.
func TestListsEndpointReturnsArrayNotNull(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Break the store so every getCachedList fails — this is the path
	// that would produce JSON null with a nil slice.
	if err := srv.CloseStore(); err != nil {
		t.Fatalf("CloseStore: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/lists", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if len(body) == 0 || body[0] != '[' {
		t.Errorf("expected JSON array starting with '[', got: %s", body)
	}
}

func TestScreenAllListsFail500(t *testing.T) {
	srv, dbPath := setupTestServer(t)
	seedDB(t, dbPath, []models.Person{
		{ID: "F1", Name: "Fail Test", ListType: models.ListOFAC},
	})

	// Break the store by closing its underlying DB connection.
	// This causes LoadCached to fail for every list type.
	if err := srv.CloseStore(); err != nil {
		t.Fatalf("CloseStore: %v", err)
	}

	body := map[string]any{
		"name":      "Fail Test",
		"threshold": 0.8,
		"lists":     []string{"OFAC"},
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/screen", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestScreenExplicitThresholdZero(t *testing.T) {
	srv, dbPath := setupTestServer(t)
	seedDB(t, dbPath, []models.Person{
		{ID: "T1", Name: "John Smith", ListType: models.ListOFAC, Nationality: "US"},
	})

	body := map[string]any{
		"name":      "Jane Doe",
		"threshold": 0,
		"lists":     []string{"OFAC"},
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/screen", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Count   int `json:"count"`
		Matches []struct {
			Score float64 `json:"score"`
		} `json:"matches"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if resp.Count < 1 {
		t.Fatalf("expected >=1 match with threshold=0, got %d. Response: %s", resp.Count, w.Body.String())
	}
	// The match score should be below 0.8, proving threshold=0 was used.
	// If the server replaced 0 with 0.8, this match would be filtered.
	for _, m := range resp.Matches {
		if m.Score >= 0.8 {
			t.Errorf("expected a match with score < 0.8 (proving threshold=0), got %.4f", m.Score)
		}
	}
}

// TestScreenOmittedThresholdDefaults verifies that when threshold is omitted
// from the JSON body, the default 0.8 applies (exact match still found).
func TestScreenOmittedThresholdDefaults(t *testing.T) {
	srv, dbPath := setupTestServer(t)
	seedDB(t, dbPath, []models.Person{
		{ID: "T2", Name: "John Smith", ListType: models.ListOFAC, Nationality: "US"},
	})

	body := map[string]any{
		"name":  "John Smith",
		"lists": []string{"OFAC"},
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/screen", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if resp.Count < 1 {
		t.Fatalf("expected >=1 exact match with default threshold, got %d", resp.Count)
	}
}

func TestBatchPanicDoesNotHang(t *testing.T) {
	srv, dbPath := setupTestServer(t)
	seedDB(t, dbPath, []models.Person{
		{ID: "P1", Name: "Panic Trigger", ListType: models.ListOFAC},
	})

	names := make([]string, 20) // > batchSequentialThreshold (8) → parallel path
	for i := range names {
		names[i] = "Test Name"
	}
	body := map[string]any{
		"names":     names,
		"threshold": 0.8,
		"lists":     []string{"OFAC"},
	}
	bodyBytes, _ := json.Marshal(body)

	done := make(chan struct{})
	go func() {
		req := httptest.NewRequest("POST", "/api/v1/screen/batch", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		done <- struct{}{}
	}()

	select {
	case <-done:
		// Response arrived — no hang. This is the expected path.
	case <-time.After(10 * time.Second):
		t.Fatal("batch request hung: goroutine panic blocked the collector loop")
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
