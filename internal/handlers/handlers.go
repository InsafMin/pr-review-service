package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"pr-review-service/internal/database"
	"pr-review-service/internal/models"
)

type Handler struct {
	db *database.DB
}

func New(db *database.DB) *Handler {
	return &Handler{db: db}
}

func (h *Handler) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *Handler) respondError(w http.ResponseWriter, status int, code, message string) {
	h.respondJSON(w, status, models.ErrorResponse{
		Error: models.ErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (h *Handler) CreateTeam(w http.ResponseWriter, r *http.Request) {
	var team models.Team
	if err := json.NewDecoder(r.Body).Decode(&team); err != nil {
		h.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	if err := h.db.CreateTeam(r.Context(), &team); err != nil {
		if strings.Contains(err.Error(), models.ErrTeamExists) {
			h.respondError(w, http.StatusBadRequest, models.ErrTeamExists, "team_name already exists")
			return
		}
		log.Printf("Error creating team: %v", err)
		h.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
		return
	}

	h.respondJSON(w, http.StatusCreated, map[string]interface{}{"team": team})
}

func (h *Handler) GetTeam(w http.ResponseWriter, r *http.Request) {
	teamName := r.URL.Query().Get("team_name")
	if teamName == "" {
		h.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "team_name is required")
		return
	}

	team, err := h.db.GetTeam(r.Context(), teamName)
	if err != nil {
		if strings.Contains(err.Error(), models.ErrNotFound) {
			h.respondError(w, http.StatusNotFound, models.ErrNotFound, "team not found")
			return
		}
		log.Printf("Error getting team: %v", err)
		h.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
		return
	}

	h.respondJSON(w, http.StatusOK, team)
}

func (h *Handler) SetUserActive(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID   string `json:"user_id"`
		IsActive bool   `json:"is_active"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	user, err := h.db.SetUserActive(r.Context(), req.UserID, req.IsActive)
	if err != nil {
		if strings.Contains(err.Error(), models.ErrNotFound) {
			h.respondError(w, http.StatusNotFound, models.ErrNotFound, "user not found")
			return
		}
		log.Printf("Error setting user active: %v", err)
		h.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
		return
	}

	h.respondJSON(w, http.StatusOK, map[string]interface{}{"user": user})
}

func (h *Handler) CreatePR(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PullRequestID   string `json:"pull_request_id"`
		PullRequestName string `json:"pull_request_name"`
		AuthorID        string `json:"author_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	pr, err := h.db.CreatePR(r.Context(), req.PullRequestID, req.PullRequestName, req.AuthorID)
	if err != nil {
		if strings.Contains(err.Error(), models.ErrPRExists) {
			h.respondError(w, http.StatusConflict, models.ErrPRExists, "PR id already exists")
			return
		}
		if strings.Contains(err.Error(), models.ErrNotFound) {
			h.respondError(w, http.StatusNotFound, models.ErrNotFound, "author or team not found")
			return
		}
		log.Printf("Error creating PR: %v", err)
		h.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
		return
	}

	h.respondJSON(w, http.StatusCreated, map[string]interface{}{"pr": pr})
}

func (h *Handler) MergePR(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PullRequestID string `json:"pull_request_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	pr, err := h.db.MergePR(r.Context(), req.PullRequestID)
	if err != nil {
		if strings.Contains(err.Error(), models.ErrNotFound) {
			h.respondError(w, http.StatusNotFound, models.ErrNotFound, "PR not found")
			return
		}
		log.Printf("Error merging PR: %v", err)
		h.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
		return
	}

	h.respondJSON(w, http.StatusOK, map[string]interface{}{"pr": pr})
}

func (h *Handler) ReassignReviewer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PullRequestID string `json:"pull_request_id"`
		OldUserID     string `json:"old_user_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	pr, replacedBy, err := h.db.ReassignReviewer(r.Context(), req.PullRequestID, req.OldUserID)
	if err != nil {
		if strings.Contains(err.Error(), models.ErrPRMerged) {
			h.respondError(w, http.StatusConflict, models.ErrPRMerged, "cannot reassign on merged PR")
			return
		}
		if strings.Contains(err.Error(), models.ErrNotAssigned) {
			h.respondError(w, http.StatusConflict, models.ErrNotAssigned, "reviewer is not assigned to this PR")
			return
		}
		if strings.Contains(err.Error(), models.ErrNoCandidate) {
			h.respondError(w, http.StatusConflict, models.ErrNoCandidate, "no active replacement candidate in team")
			return
		}
		if strings.Contains(err.Error(), models.ErrNotFound) {
			h.respondError(w, http.StatusNotFound, models.ErrNotFound, "PR or user not found")
			return
		}
		log.Printf("Error reassigning reviewer: %v", err)
		h.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
		return
	}

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"pr":          pr,
		"replaced_by": replacedBy,
	})
}

func (h *Handler) GetUserReviews(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		h.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "user_id is required")
		return
	}

	prs, err := h.db.GetUserReviews(r.Context(), userID)
	if err != nil {
		log.Printf("Error getting user reviews: %v", err)
		h.respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
		return
	}

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"user_id":       userID,
		"pull_requests": prs,
	})
}
