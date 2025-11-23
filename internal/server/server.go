package server

import (
	"log"
	"net/http"
	"time"

	"pr-review-service/internal/handlers"
)

type Server struct {
	handler *handlers.Handler
	mux     *http.ServeMux
}

func New(handler *handlers.Handler) *Server {
	s := &Server{
		handler: handler,
		mux:     http.NewServeMux(),
	}
	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	s.mux.HandleFunc("/health", s.handler.Health)

	s.mux.HandleFunc("/team/add", s.methodFilter(http.MethodPost, s.handler.CreateTeam))
	s.mux.HandleFunc("/team/get", s.methodFilter(http.MethodGet, s.handler.GetTeam))

	s.mux.HandleFunc("/users/setIsActive", s.methodFilter(http.MethodPost, s.handler.SetUserActive))
	s.mux.HandleFunc("/users/getReview", s.methodFilter(http.MethodGet, s.handler.GetUserReviews))

	s.mux.HandleFunc("/pullRequest/create", s.methodFilter(http.MethodPost, s.handler.CreatePR))
	s.mux.HandleFunc("/pullRequest/merge", s.methodFilter(http.MethodPost, s.handler.MergePR))
	s.mux.HandleFunc("/pullRequest/reassign", s.methodFilter(http.MethodPost, s.handler.ReassignReviewer))
}

func (s *Server) methodFilter(method string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		next(w, r)
	}
}

func (s *Server) Start(port string) error {
	addr := ":" + port
	log.Printf("Server starting on %s", addr)

	server := &http.Server{
		Addr:         addr,
		Handler:      s.loggingMiddleware(s.mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return server.ListenAndServe()
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.RequestURI, time.Since(start))
	})
}
