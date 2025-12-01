// internal/catalog/handler.go
package catalog

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

func (h *Handler) HandleItems(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.handleAddItem(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) HandleItem(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/items/")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid item ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.handleGetItem(w, r, id)
	case http.MethodPatch:
		h.handleUpdateItemCopies(w, r, id)
	case http.MethodDelete:
		h.handleRemoveItem(w, r, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) HandleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "missing search query", http.StatusBadRequest)
		return
	}

	items, err := h.service.Search(r.Context(), query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(items)
}

func (h *Handler) handleAddItem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ISBN        string `json:"isbn"`
		Title       string `json:"title"`
		Author      string `json:"author"`
		TotalCopies int    `json:"total_copies"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	item, err := h.service.AddItem(r.Context(), req.ISBN, req.Title, req.Author, req.TotalCopies)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(item)
}

func (h *Handler) handleGetItem(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	item, err := h.service.GetItem(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(item)
}

func (h *Handler) handleUpdateItemCopies(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	var req struct {
		TotalCopies int `json:"total_copies"`
		Available   int `json:"available"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.service.UpdateItemCopies(r.Context(), id, req.TotalCopies, req.Available); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleRemoveItem(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	if err := h.service.RemoveItem(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
