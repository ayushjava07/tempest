package api

import (
	"encoding/json"
	"net/http"
	"strings"

	pkgerrors "github.com/tempest-io/tempest/pkg/errors"
)

type problemResponse struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail"`
	Instance string `json:"instance,omitempty"`
}

func writeProblem(w http.ResponseWriter, r *http.Request, code int, title, detail string) {
	pr := problemResponse{
		Type:     "about:blank",
		Title:    title,
		Status:   code,
		Detail:   detail,
		Instance: r.URL.Path,
	}
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(pr)
}

func mapError(w http.ResponseWriter, r *http.Request, err error) {
	if pkgerrors.Is(err, pkgerrors.ErrNotFound) {
		writeProblem(w, r, http.StatusNotFound, "Not Found", strings.TrimPrefix(err.Error(), "not_found: "))
		return
	}
	if pkgerrors.Is(err, pkgerrors.ErrConflict) {
		writeProblem(w, r, http.StatusConflict, "Conflict", strings.TrimPrefix(err.Error(), "conflict: "))
		return
	}
	if pkgerrors.Is(err, pkgerrors.ErrUnauthenticated) {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "authentication required")
		return
	}
	if pkgerrors.Is(err, pkgerrors.ErrPermissionDenied) {
		writeProblem(w, r, http.StatusForbidden, "Forbidden", "insufficient permissions")
		return
	}
	if pkgerrors.Is(err, pkgerrors.ErrInvalidArgument) {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "an internal error occurred")
}
