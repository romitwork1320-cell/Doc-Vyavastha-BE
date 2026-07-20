// Package apiresp implements the response envelope the Angular frontend expects
// for every endpoint.
//
// Wire contract (do not change without coordinating with the FE):
//
//	ApiResponse<T> = { data: T|null, success: bool, message: string, statusCode: number }
//
// List endpoints additionally carry a top-level total count. Different FE
// services read different field names (`totalCount` in the student-domain
// services, `totalRecords` in the platform services), so list responses
// populate BOTH with the same value.
//
// Status-code convention (matches the original ASP.NET backend + the FE mocks):
//   - Handled outcomes — including logical 201/400/404/409 — are returned as
//     HTTP 200 with the real outcome carried in the envelope's `statusCode`.
//     The FE delivers these through its normal `next` handler (its mocked
//     services resolve 404s as successful observables carrying statusCode:404).
//   - Only the JWT auth boundary returns a real HTTP 401, because the FE's
//     interceptor keys its single-flight token-refresh off `error.status === 401`.
//   - Unexpected server faults return a real HTTP 500.
package apiresp

import (
	"encoding/json"
	"net/http"
)

// Envelope is the universal response body.
type Envelope struct {
	Data         any    `json:"data"`
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	StatusCode   int    `json:"statusCode"`
	TotalCount   *int64 `json:"totalCount,omitempty"`
	TotalRecords *int64 `json:"totalRecords,omitempty"`
}

// write marshals env and writes it with the given HTTP status.
func write(w http.ResponseWriter, httpStatus int, env Envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(httpStatus)
	_ = json.NewEncoder(w).Encode(env)
}

// OK returns a successful 200 response.
func OK(w http.ResponseWriter, data any, message string) {
	if message == "" {
		message = "Success"
	}
	write(w, http.StatusOK, Envelope{Data: data, Success: true, Message: message, StatusCode: http.StatusOK})
}

// Created returns a logical 201 (carried in the envelope; HTTP stays 200).
func Created(w http.ResponseWriter, data any, message string) {
	if message == "" {
		message = "Created"
	}
	write(w, http.StatusOK, Envelope{Data: data, Success: true, Message: message, StatusCode: http.StatusCreated})
}

// List returns a successful paginated/list response. `items` should be a
// non-nil slice so it serializes as [] rather than null when empty.
func List(w http.ResponseWriter, items any, total int64, message string) {
	if message == "" {
		message = "Success"
	}
	t := total
	write(w, http.StatusOK, Envelope{
		Data:         items,
		Success:      true,
		Message:      message,
		StatusCode:   http.StatusOK,
		TotalCount:   &t,
		TotalRecords: &t,
	})
}

// Fail returns a handled business failure: HTTP 200 with success=false and the
// logical status code in the envelope. Use for validation/not-found/conflict.
func Fail(w http.ResponseWriter, logicalStatus int, message string) {
	write(w, http.StatusOK, Envelope{Data: nil, Success: false, Message: message, StatusCode: logicalStatus})
}

// BadRequest is a 400 business failure.
func BadRequest(w http.ResponseWriter, message string) {
	if message == "" {
		message = "Bad request"
	}
	Fail(w, http.StatusBadRequest, message)
}

// NotFound is a 404 business failure.
func NotFound(w http.ResponseWriter, message string) {
	if message == "" {
		message = "Not found"
	}
	Fail(w, http.StatusNotFound, message)
}

// Conflict is a 409 business failure (e.g. duplicate).
func Conflict(w http.ResponseWriter, message string) {
	if message == "" {
		message = "Conflict"
	}
	Fail(w, http.StatusConflict, message)
}

// Forbidden is a 403 business failure (authenticated but not permitted).
func Forbidden(w http.ResponseWriter, message string) {
	if message == "" {
		message = "Forbidden"
	}
	Fail(w, http.StatusForbidden, message)
}

// Unauthorized returns a REAL HTTP 401 so the FE interceptor triggers its
// token-refresh flow. Use only from the auth middleware / token validation.
func Unauthorized(w http.ResponseWriter, message string) {
	if message == "" {
		message = "Unauthorized"
	}
	write(w, http.StatusUnauthorized, Envelope{Data: nil, Success: false, Message: message, StatusCode: http.StatusUnauthorized})
}

// ServerError returns a real HTTP 500 for unexpected faults.
func ServerError(w http.ResponseWriter, message string) {
	if message == "" {
		message = "Internal server error"
	}
	write(w, http.StatusInternalServerError, Envelope{Data: nil, Success: false, Message: message, StatusCode: http.StatusInternalServerError})
}
