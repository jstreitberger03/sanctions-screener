package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jstreitberger03/sanctions-screener/pkg/ingest"
	"github.com/jstreitberger03/sanctions-screener/pkg/models"
	"github.com/jstreitberger03/sanctions-screener/pkg/screening"
)

type Config struct {
	Port   int
	DBPath string
}

type Server struct {
	router *chi.Mux
	store  *ingest.Store
	port   int
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

	s := &Server{store: store, port: port}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

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
	return http.ListenAndServe(fmt.Sprintf(":%d", s.port), s.router)
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
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	matches := screening.Screen(req.Name, persons, req.Threshold)

	resp := screenResponse{
		InputName:     req.Name,
		ScreeningTime: time.Since(start).Milliseconds(),
		Count:         len(matches),
	}

	for _, m := range matches {
		resp.Matches = append(resp.Matches, matchResponse{
			PersonID:    m.Person.ID,
			Name:        m.Person.Name,
			Score:       m.Score,
			MatchType:   string(m.MatchType),
			List:        string(m.Person.ListType),
			Nationality: m.Person.Nationality,
		})
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
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var results []screenResponse
	totalMatches := 0

	for _, name := range req.Names {
		matches := screening.Screen(name, persons, req.Threshold)
		totalMatches += len(matches)

		sr := screenResponse{
			InputName:     name,
			ScreeningTime: 0,
			Count:         len(matches),
		}

		for _, m := range matches {
			sr.Matches = append(sr.Matches, matchResponse{
				PersonID:    m.Person.ID,
				Name:        m.Person.Name,
				Score:       m.Score,
				MatchType:   string(m.MatchType),
				List:        string(m.Person.ListType),
				Nationality: m.Person.Nationality,
			})
		}

		results = append(results, sr)
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
		persons, err := s.store.LoadCached(lt)
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

	persons, err := s.store.LoadCached(listType)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"list":  id,
		"count": len(persons),
	})
}

func (s *Server) loadLists(listNames []string) ([]models.Person, error) {
	var all []models.Person

	for _, name := range listNames {
		persons, err := s.store.LoadCached(models.ListType(name))
		if err != nil {
			return nil, fmt.Errorf("load list %s: %w", name, err)
		}
		all = append(all, persons...)
	}

	if len(all) == 0 {
		all, _ = s.store.LoadCached(models.ListOFAC)
	}

	return all, nil
}
