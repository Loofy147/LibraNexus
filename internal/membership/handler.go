// internal/membership/handler.go
package membership

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) HandleMembers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.handleRegisterMember(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) HandleMember(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/members/")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid member ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.handleGetMember(w, r, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	member, err := h.service.Authenticate(r.Context(), req.Email, req.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// In a real application, you would return a JWT here.
	json.NewEncoder(w).Encode(member)
}

func (h *Handler) handleRegisterMember(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	member, err := h.service.RegisterMember(r.Context(), req.Email, req.Name, req.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(member)
}

func (h *Handler) handleGetMember(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	member, err := h.service.GetMember(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(member)
}
