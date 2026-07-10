package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jstreitberger03/sanctions-screener/pkg/ingest"
	"github.com/jstreitberger03/sanctions-screener/pkg/models"
)

func seedDB(t *testing.T, dbPath string, persons []models.Person) {
	t.Helper()
	store, err := ingest.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	// Write persons to a temp JSON file and use ImportJSON (exported).
	f, err := os.CreateTemp("", "seed-*.json")
	if err != nil {
		t.Fatalf("create temp seed file: %v", err)
	}
	data, err := json.Marshal(persons)
	if err != nil {
		t.Fatalf("marshal persons: %v", err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	f.Close()
	defer os.Remove(f.Name())

	if _, err := store.ImportJSON(f.Name()); err != nil {
		t.Fatalf("ImportJSON: %v", err)
	}
}

// TestCacheReturnsConsistentResults verifies that repeated requests return
// the same data, confirming the cache is populated and serving correctly.
func TestCacheReturnsConsistentResults(t *testing.T) {
	srv, dbPath := setupTestServer(t)
	seedDB(t, dbPath, []models.Person{
		{ID: "SDN-001", Name: "Mohammed Al-Rashid", ListType: models.ListOFAC, Nationality: "SY"},
		{ID: "SDN-002", Name: "John Smith", ListType: models.ListOFAC, Nationality: "US"},
	})

	body := map[string]any{
		"name":      "John Smith",
		"threshold": 0.8,
		"lists":     []string{"OFAC"},
	}
	bodyBytes, _ := json.Marshal(body)

	// First request — populates the cache.
	req1 := httptest.NewRequest("POST", "/api/v1/screen", bytes.NewReader(bodyBytes))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	srv.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Fatalf("request 1: expected 200, got %d: %s", w1.Code, w1.Body.String())
	}

	var resp1 struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(w1.Body.Bytes(), &resp1); err != nil {
		t.Fatalf("parse response 1: %v", err)
	}
	if resp1.Count == 0 {
		t.Fatalf("expected matches on first request, got 0. Response: %s", w1.Body.String())
	}

	// Second request — should return identical results (served from cache).
	req2 := httptest.NewRequest("POST", "/api/v1/screen", bytes.NewReader(bodyBytes))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("request 2: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var resp2 struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("parse response 2: %v", err)
	}

	if resp1.Count != resp2.Count {
		t.Errorf("cache inconsistency: request 1 count=%d, request 2 count=%d", resp1.Count, resp2.Count)
	}
}

// TestLoadListsDefaultsToAllLists verifies that when no lists are specified
// in the request, the server loads all available lists instead of silently
// falling back to just OFAC.
func TestLoadListsDefaultsToAllLists(t *testing.T) {
	srv, dbPath := setupTestServer(t)
	seedDB(t, dbPath, []models.Person{
		{ID: "OFAC-1", Name: "John Smith", ListType: models.ListOFAC},
		{ID: "EU-1", Name: "Jane Smith", ListType: models.ListEU},
	})

	// Request with empty lists — should query all three.
	body := map[string]any{
		"name":      "John Smith",
		"threshold": 0.8,
		"lists":     []string{},
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
		Matches []struct {
			List string `json:"list"`
		} `json:"matches"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}

	if resp.Count == 0 {
		t.Fatalf("expected matches when lists defaults to all, got 0. Response: %s", w.Body.String())
	}

	// Both OFAC and EU matches should be present.
	lists := make(map[string]bool)
	for _, m := range resp.Matches {
		lists[m.List] = true
	}
	if !lists["OFAC"] {
		t.Error("expected OFAC matches when lists is empty (defaults to all)")
	}
	if !lists["EU"] {
		t.Error("expected EU matches when lists is empty (defaults to all)")
	}
}

// TestLoadListsSkipsUnknown verifies that an unknown list name in the request
// does not cause a 500 error — it is silently skipped.
func TestLoadListsSkipsUnknown(t *testing.T) {
	srv, dbPath := setupTestServer(t)
	seedDB(t, dbPath, []models.Person{
		{ID: "SDN-001", Name: "Mohammed Al-Rashid", ListType: models.ListOFAC, Nationality: "SY"},
	})

	body := map[string]any{
		"name":      "John Smith",
		"threshold": 0.8,
		"lists":     []string{"OFAC", "NONEXISTENT"},
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/screen", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	// Must return 200, not 500. The unknown list is skipped gracefully.
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (unknown list skipped), got %d: %s", w.Code, w.Body.String())
	}
}

// TestCacheEmptyWhenNoData verifies that an empty DB returns 0 matches
// without errors.
func TestCacheEmptyWhenNoData(t *testing.T) {
	srv, _ := setupTestServer(t)

	body := map[string]any{
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
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Count int `json:"count"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp) //nolint:errcheck // test: checking count, not parse error
	if resp.Count != 0 {
		t.Errorf("expected 0 matches on empty DB, got %d", resp.Count)
	}
}

// TestListsEndpointReflectsCachedData verifies that the /lists endpoint
// returns the correct counts for each list type after seeding.
func TestListsEndpointReflectsCachedData(t *testing.T) {
	srv, dbPath := setupTestServer(t)
	seedDB(t, dbPath, []models.Person{
		{ID: "OFAC-1", Name: "Person A", ListType: models.ListOFAC},
		{ID: "OFAC-2", Name: "Person B", ListType: models.ListOFAC},
		{ID: "EU-1", Name: "Person C", ListType: models.ListEU},
	})

	req := httptest.NewRequest("GET", "/api/v1/lists", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var entries []struct {
		ID    string `json:"id"`
		Count int    `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &entries); err != nil {
		t.Fatalf("parse lists response: %v", err)
	}

	counts := make(map[string]int)
	for _, e := range entries {
		counts[e.ID] = e.Count
	}
	if counts["OFAC"] != 2 {
		t.Errorf("expected OFAC count=2, got %d", counts["OFAC"])
	}
	if counts["EU"] != 1 {
		t.Errorf("expected EU count=1, got %d", counts["EU"])
	}
}
