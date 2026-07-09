package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jstreitberger03/sanctions-screener/pkg/ingest"
	"github.com/jstreitberger03/sanctions-screener/pkg/models"
	"github.com/jstreitberger03/sanctions-screener/pkg/screening"
)

type Config struct {
	Port   int
	DBPath string
}

type Server struct {
	router      *chi.Mux
	store       *ingest.Store
	port        int
	cacheMu     sync.RWMutex
	personCache map[models.ListType][]models.Person
}

func New(cfg Config) (*Server, error) {
	store, err := ingest.NewStore(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}

	port := cfg.Port
	if port == 0 {
		port = 8080
	}

	s := &Server{store: store, port: port, personCache: make(map[models.ListType][]models.Person)}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(middleware.RequestSize(1 << 20))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", s.handleHealth)
		r.Post("/screen", s.handleScreen)
		r.Post("/screen/batch", s.handleScreenBatch)
		r.Get("/lists", s.handleLists)
		r.Get("/lists/{id}/count", s.handleListCount)
	})

	s.router = r
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) ListenAndServe() error {
	defer s.store.Close()

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: s.router,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	case sig := <-quit:
		fmt.Printf("\nshutting down (%v)...\n", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	}
}

type screenRequest struct {
	Name      string   `json:"name"`
	Threshold float64  `json:"threshold"`
	Lists     []string `json:"lists"`
}

type matchResponse struct {
	PersonID    string  `json:"person_id"`
	Name        string  `json:"name"`
	Score       float64 `json:"score"`
	MatchType   string  `json:"match_type"`
	List        string  `json:"list"`
	Nationality string  `json:"nationality"`
}

type screenResponse struct {
	Matches        []matchResponse `json:"matches"`
	ScreeningTime  int64           `json:"screening_time_ms"`
	InputName      string          `json:"input_name"`
	Count          int             `json:"count"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleScreen(w http.ResponseWriter, r *http.Request) {
	var req screenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	if req.Threshold == 0 {
		req.Threshold = 0.8
	}

	start := time.Now()
	persons, err := s.loadLists(req.Lists)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load sanctions lists")
		return
	}

	matches := screening.Screen(req.Name, persons, req.Threshold)

	resp := screenResponse{
		InputName:     req.Name,
		ScreeningTime: time.Since(start).Milliseconds(),
		Count:         len(matches),
		Matches:       toMatchResponses(matches),
	}

	writeJSON(w, http.StatusOK, resp)
}

type batchRequest struct {
	Names     []string `json:"names"`
	Threshold float64  `json:"threshold"`
	Lists     []string `json:"lists"`
}

type batchResponse struct {
	Results       []screenResponse `json:"results"`
	ScreeningTime int64            `json:"screening_time_ms"`
	TotalMatches  int              `json:"total_matches"`
}

type batchResult struct {
	index   int
	matches []models.Match
}

func (s *Server) handleScreenBatch(w http.ResponseWriter, r *http.Request) {
	var req batchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Names) == 0 {
		writeError(w, http.StatusBadRequest, "names array is required")
		return
	}

	if req.Threshold == 0 {
		req.Threshold = 0.8
	}

	start := time.Now()
	persons, err := s.loadLists(req.Lists)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load sanctions lists")
		return
	}

	// Screen names concurrently with a bounded worker pool.
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	ch := make(chan batchResult, len(req.Names))

	for i, name := range req.Names {
		sem <- struct{}{}
		go func(idx int, n string) {
			defer func() { <-sem }()
			ch <- batchResult{index: idx, matches: screening.Screen(n, persons, req.Threshold)}
		}(i, name)
	}

	results := make([]screenResponse, len(req.Names))
	totalMatches := 0
	for range req.Names {
		r := <-ch
		totalMatches += len(r.matches)
		results[r.index] = screenResponse{
			InputName: req.Names[r.index],
			Count:     len(r.matches),
			Matches:   toMatchResponses(r.matches),
		}
	}

	resp := batchResponse{
		Results:       results,
		ScreeningTime: time.Since(start).Milliseconds(),
		TotalMatches:  totalMatches,
	}

	writeJSON(w, http.StatusOK, resp)
}

type listEntry struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func (s *Server) handleLists(w http.ResponseWriter, r *http.Request) {
	lists := []models.ListType{models.ListOFAC, models.ListEU, models.ListUN}
	var entries []listEntry

	for _, lt := range lists {
		persons, err := s.getCachedList(lt)
		if err != nil {
			continue
		}
		entries = append(entries, listEntry{
			ID:    string(lt),
			Name:  string(lt),
			Count: len(persons),
		})
	}

	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) handleListCount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	listType := models.ListType(id)

	persons, err := s.getCachedList(listType)
	if err != nil {
		writeError(w, http.StatusNotFound, "list not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"list":  id,
		"count": len(persons),
	})
}

func toMatchResponses(matches []models.Match) []matchResponse {
	resp := make([]matchResponse, 0, len(matches))
	for _, m := range matches {
		resp = append(resp, matchResponse{
			PersonID:    m.Person.ID,
			Name:        m.Person.Name,
			Score:       m.Score,
			MatchType:   string(m.MatchType),
			List:        string(m.Person.ListType),
			Nationality: m.Person.Nationality,
		})
	}
	return resp
}

func (s *Server) getCachedList(lt models.ListType) ([]models.Person, error) {
	s.cacheMu.RLock()
	cached, ok := s.personCache[lt]
	s.cacheMu.RUnlock()

	if ok {
		return cached, nil
	}

	persons, err := s.store.LoadCached(lt)
	if err != nil {
		return nil, err
	}

	s.cacheMu.Lock()
	s.personCache[lt] = persons
	s.cacheMu.Unlock()

	return persons, nil
}

func (s *Server) loadLists(listNames []string) ([]models.Person, error) {
	var all []models.Person

	for _, name := range listNames {
		persons, err := s.getCachedList(models.ListType(name))
		if err != nil {
			return nil, fmt.Errorf("load list %s: %w", name, err)
		}
		all = append(all, persons...)
	}

	if len(all) == 0 {
		persons, err := s.getCachedList(models.ListOFAC)
		if err != nil {
			return nil, fmt.Errorf("load default list: %w", err)
		}
		return persons, nil
	}

	return all, nil
}
