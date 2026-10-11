package problem

import (
	"encoding/json"
	"net/http"
)

type body struct {
	Type             string         `json:"type"`
	Title            string         `json:"title"`
	Status           int            `json:"status"`
	Detail           string         `json:"detail,omitempty"`
	InvalidParams    []InvalidParam `json:"invalid_params,omitempty"`
	BlockingBuildIDs []string       `json:"blocking_build_ids,omitempty"`
}

// InvalidParam is one field-level violation reported in a validation
// problem's invalid_params member (RFC 9457 §3.2).
type InvalidParam struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// Write writes an RFC 9457 application/problem+json response with the given
// status, title, and detail. problemType should be a stable URI identifying
// the problem category (e.g. "https://mykeebs.info/errors/internal-error").
func Write(w http.ResponseWriter, status int, problemType, title, detail string) {
	writeBody(w, status, body{
		Type:   problemType,
		Title:  title,
		Status: status,
		Detail: detail,
	})
}

// ValidationFailed writes a 400 Problem response listing every field-level
// violation in invalidParams, per RFC 9457 §3.2's invalid_params member.
func ValidationFailed(w http.ResponseWriter, detail string, invalidParams []InvalidParam) {
	writeBody(w, http.StatusBadRequest, body{
		Type:          "https://mykeebs.info/errors/bad-request",
		Title:         "Bad Request",
		Status:        http.StatusBadRequest,
		Detail:        detail,
		InvalidParams: invalidParams,
	})
}

func writeBody(w http.ResponseWriter, status int, b body) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(b)
}

// NotFound writes a 404 Problem response.
func NotFound(w http.ResponseWriter, detail string) {
	Write(w, http.StatusNotFound, "https://mykeebs.info/errors/not-found", "Not Found", detail)
}

// Internal writes a 500 Problem response.
func Internal(w http.ResponseWriter, detail string) {
	Write(w, http.StatusInternalServerError, "https://mykeebs.info/errors/internal-error", "Internal Server Error", detail)
}

// Forbidden writes a 403 Problem response.
func Forbidden(w http.ResponseWriter, detail string) {
	Write(w, http.StatusForbidden, "https://mykeebs.info/errors/forbidden", "Forbidden", detail)
}

// Unauthorized writes a 401 Problem response.
func Unauthorized(w http.ResponseWriter, detail string) {
	Write(w, http.StatusUnauthorized, "https://mykeebs.info/errors/unauthorized", "Unauthorized", detail)
}

// Conflict writes a 409 Problem response.
func Conflict(w http.ResponseWriter, detail string) {
	Write(w, http.StatusConflict, "https://mykeebs.info/errors/conflict", "Conflict", detail)
}

// UsernameUnavailable writes a 409 Problem response for a create/update
// whose requested username is already taken by another user. Unlike every
// other 409 in this package it uses a distinct type URI, not
// .../errors/conflict, so the frontend can branch on it to render the error
// under the username field.
func UsernameUnavailable(w http.ResponseWriter, detail string) {
	Write(w, http.StatusConflict,
		"https://mykeebs.info/errors/username-unavailable",
		"Username Unavailable", detail)
}

// StillReferenced writes a 409 Problem response listing the ids of every
// build blocking a delete in blocking_build_ids, per RFC 9457 §3.2's
// pattern of extending Problem with an extension member (mirrors
// ValidationFailed's invalid_params for the analogous validation case).
func StillReferenced(w http.ResponseWriter, detail string, blockingBuildIDs []string) {
	writeBody(w, http.StatusConflict, body{
		Type:             "https://mykeebs.info/errors/conflict",
		Title:            "Conflict",
		Status:           http.StatusConflict,
		Detail:           detail,
		BlockingBuildIDs: blockingBuildIDs,
	})
}

// BadRequest writes a 400 Problem response.
func BadRequest(w http.ResponseWriter, detail string) {
	Write(w, http.StatusBadRequest, "https://mykeebs.info/errors/bad-request", "Bad Request", detail)
}

// TooManyRequests writes a 429 Problem response.
func TooManyRequests(w http.ResponseWriter, detail string) {
	Write(w, http.StatusTooManyRequests, "https://mykeebs.info/errors/too-many-requests", "Too Many Requests", detail)
}
