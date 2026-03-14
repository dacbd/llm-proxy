package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateStack_EmptyStack(t *testing.T) {
	// An empty stack should return the handler unchanged
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("final handler"))
	})

	stack := CreateStack()
	handler := stack(finalHandler)

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}
	if rr.Body.String() != "final handler" {
		t.Errorf("Expected body 'final handler', got '%s'", rr.Body.String())
	}
}

func TestCreateStack_SingleMiddleware(t *testing.T) {
	var executionOrder []string

	middleware1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			executionOrder = append(executionOrder, "middleware1-before")
			next.ServeHTTP(w, r)
			executionOrder = append(executionOrder, "middleware1-after")
		})
	}

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		executionOrder = append(executionOrder, "handler")
		w.WriteHeader(http.StatusOK)
	})

	stack := CreateStack(middleware1)
	handler := stack(finalHandler)

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	expectedOrder := []string{"middleware1-before", "handler", "middleware1-after"}
	if len(executionOrder) != len(expectedOrder) {
		t.Errorf("Expected %d execution steps, got %d", len(expectedOrder), len(executionOrder))
	}

	for i, expected := range expectedOrder {
		if executionOrder[i] != expected {
			t.Errorf("Step %d: expected '%s', got '%s'", i, expected, executionOrder[i])
		}
	}
}

func TestCreateStack_MultipleMiddleware_ExecutionOrder(t *testing.T) {
	var executionOrder []string

	middleware1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			executionOrder = append(executionOrder, "middleware1-before")
			next.ServeHTTP(w, r)
			executionOrder = append(executionOrder, "middleware1-after")
		})
	}

	middleware2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			executionOrder = append(executionOrder, "middleware2-before")
			next.ServeHTTP(w, r)
			executionOrder = append(executionOrder, "middleware2-after")
		})
	}

	middleware3 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			executionOrder = append(executionOrder, "middleware3-before")
			next.ServeHTTP(w, r)
			executionOrder = append(executionOrder, "middleware3-after")
		})
	}

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		executionOrder = append(executionOrder, "handler")
		w.WriteHeader(http.StatusOK)
	})

	// CreateStack should execute middleware in the order they're passed
	stack := CreateStack(middleware1, middleware2, middleware3)
	handler := stack(finalHandler)

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Expected: middleware execute in order (1->2->3->handler->3->2->1)
	expectedOrder := []string{
		"middleware1-before",
		"middleware2-before",
		"middleware3-before",
		"handler",
		"middleware3-after",
		"middleware2-after",
		"middleware1-after",
	}

	if len(executionOrder) != len(expectedOrder) {
		t.Errorf("Expected %d execution steps, got %d", len(expectedOrder), len(executionOrder))
	}

	for i, expected := range expectedOrder {
		if i >= len(executionOrder) {
			t.Errorf("Missing step %d: expected '%s'", i, expected)
			continue
		}
		if executionOrder[i] != expected {
			t.Errorf("Step %d: expected '%s', got '%s'", i, expected, executionOrder[i])
		}
	}
}

func TestCreateStack_MiddlewareCanModifyRequest(t *testing.T) {
	addHeaderMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Header.Set("X-Custom-Header", "added-by-middleware")
			next.ServeHTTP(w, r)
		})
	}

	var receivedHeader string
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get("X-Custom-Header")
		w.WriteHeader(http.StatusOK)
	})

	stack := CreateStack(addHeaderMiddleware)
	handler := stack(finalHandler)

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if receivedHeader != "added-by-middleware" {
		t.Errorf("Expected header 'added-by-middleware', got '%s'", receivedHeader)
	}
}

func TestCreateStack_MiddlewareCanModifyResponse(t *testing.T) {
	addResponseHeaderMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Response-Header", "from-middleware")
			next.ServeHTTP(w, r)
		})
	}

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("response body"))
	})

	stack := CreateStack(addResponseHeaderMiddleware)
	handler := stack(finalHandler)

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Header().Get("X-Response-Header") != "from-middleware" {
		t.Errorf("Expected response header 'from-middleware', got '%s'", rr.Header().Get("X-Response-Header"))
	}
}

func TestCreateStack_MiddlewareCanShortCircuit(t *testing.T) {
	var handlerCalled bool

	authMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Simulate authentication failure
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte("unauthorized"))
				return // Short-circuit: don't call next
			}
			next.ServeHTTP(w, r)
		})
	}

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	stack := CreateStack(authMiddleware)
	handler := stack(finalHandler)

	// Test without authorization
	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if handlerCalled {
		t.Error("Final handler should not be called when middleware short-circuits")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", rr.Code)
	}
	if rr.Body.String() != "unauthorized" {
		t.Errorf("Expected body 'unauthorized', got '%s'", rr.Body.String())
	}
}

func TestCreateStack_MiddlewareWithAuthorization_AllowsThrough(t *testing.T) {
	var handlerCalled bool

	authMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	stack := CreateStack(authMiddleware)
	handler := stack(finalHandler)

	// Test with authorization
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer token")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("Final handler should be called when auth succeeds")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}
}

func TestCreateStack_ComplexChain(t *testing.T) {
	var executionLog []string

	// Logging middleware
	loggingMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			executionLog = append(executionLog, "logging-start")
			next.ServeHTTP(w, r)
			executionLog = append(executionLog, "logging-end")
		})
	}

	// Auth middleware
	authMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			executionLog = append(executionLog, "auth-check")
			if r.Header.Get("X-Auth") != "valid" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	// Rate limiting middleware
	rateLimitMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			executionLog = append(executionLog, "rate-limit-check")
			next.ServeHTTP(w, r)
		})
	}

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		executionLog = append(executionLog, "handler-executed")
		w.WriteHeader(http.StatusOK)
	})

	// Stack: Logging -> Auth -> RateLimit -> Handler
	stack := CreateStack(loggingMiddleware, authMiddleware, rateLimitMiddleware)
	handler := stack(finalHandler)

	// Test with valid auth
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Auth", "valid")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	expectedLog := []string{
		"logging-start",
		"auth-check",
		"rate-limit-check",
		"handler-executed",
		"logging-end",
	}

	if len(executionLog) != len(expectedLog) {
		t.Errorf("Expected %d log entries, got %d", len(expectedLog), len(executionLog))
	}

	for i, expected := range expectedLog {
		if i >= len(executionLog) {
			t.Errorf("Missing log entry %d: expected '%s'", i, expected)
			continue
		}
		if executionLog[i] != expected {
			t.Errorf("Log entry %d: expected '%s', got '%s'", i, expected, executionLog[i])
		}
	}
}
