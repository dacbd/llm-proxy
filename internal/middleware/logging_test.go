package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLogging_CallsNextHandler(t *testing.T) {
	// Track if next handler was called
	nextHandlerCalled := false

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextHandlerCalled = true
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("test response"))
	})

	handler := Logging(nextHandler)

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !nextHandlerCalled {
		t.Error("Expected next handler to be called")
	}

	if rr.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", rr.Code)
	}

	if rr.Body.String() != "test response" {
		t.Errorf("Expected body 'test response', got '%s'", rr.Body.String())
	}
}

func TestLogging_CapturesStatusCodes(t *testing.T) {
	tests := []struct {
		name           string
		handlerStatus  int
		expectedStatus int
	}{
		{
			name:           "OK status",
			handlerStatus:  http.StatusOK,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Created status",
			handlerStatus:  http.StatusCreated,
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "Not Found status",
			handlerStatus:  http.StatusNotFound,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Internal Server Error status",
			handlerStatus:  http.StatusInternalServerError,
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "Unauthorized status",
			handlerStatus:  http.StatusUnauthorized,
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.handlerStatus)
			})

			handler := Logging(nextHandler)
			req := httptest.NewRequest("GET", "/test", nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
			}
		})
	}
}

func TestLogging_DefaultStatusCode(t *testing.T) {
	// When WriteHeader is not called, it should default to 200 OK
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Don't call WriteHeader - should default to 200
		w.Write([]byte("response"))
	})

	handler := Logging(nextHandler)
	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected default status 200, got %d", rr.Code)
	}
}

func TestLogging_OutputsLogEntry(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	// Replace default logger temporarily
	oldLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(oldLogger)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	handler := Logging(nextHandler)
	req := httptest.NewRequest("POST", "/api/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	logOutput := buf.String()

	// Verify log contains expected information
	if !strings.Contains(logOutput, "Request completed") {
		t.Error("Log should contain 'Request completed'")
	}
	if !strings.Contains(logOutput, "POST") {
		t.Error("Log should contain request method 'POST'")
	}
	if !strings.Contains(logOutput, "/api/test") {
		t.Error("Log should contain request path '/api/test'")
	}
	if !strings.Contains(logOutput, "200") {
		t.Error("Log should contain status code 200")
	}
	if !strings.Contains(logOutput, "Duration") {
		t.Error("Log should contain 'Duration' field")
	}
}

func TestLogging_DifferentHTTPMethods(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			// Capture log output
			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, nil))

			oldLogger := slog.Default()
			slog.SetDefault(logger)
			defer slog.SetDefault(oldLogger)

			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			handler := Logging(nextHandler)
			req := httptest.NewRequest(method, "/test", nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			logOutput := buf.String()
			if !strings.Contains(logOutput, method) {
				t.Errorf("Log should contain HTTP method '%s'", method)
			}
		})
	}
}

func TestLogging_PreservesResponseBody(t *testing.T) {
	expectedBody := "Hello, World!"

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(expectedBody))
	})

	handler := Logging(nextHandler)
	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Body.String() != expectedBody {
		t.Errorf("Expected body '%s', got '%s'", expectedBody, rr.Body.String())
	}

	if rr.Header().Get("Content-Type") != "text/plain" {
		t.Errorf("Expected Content-Type header to be preserved")
	}
}

func TestWrappedResponseWriter_WriteHeader(t *testing.T) {
	rr := httptest.NewRecorder()
	wrw := &wrappedResponseWriter{
		ResponseWriter: rr,
		statusCode:     http.StatusOK,
	}

	wrw.WriteHeader(http.StatusNotFound)

	if wrw.statusCode != http.StatusNotFound {
		t.Errorf("Expected wrapped status code 404, got %d", wrw.statusCode)
	}

	if rr.Code != http.StatusNotFound {
		t.Errorf("Expected underlying response recorder status 404, got %d", rr.Code)
	}
}
