package server

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPrepareInfrastructureIsNotReportedAsBadPlan(t *testing.T) {
	app, _ := setup(t)
	app.Engine.Worker.Python = "/nonexistent/seven-eval-python"
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/runs", bytes.NewBufferString("{\"plan\":\"smoke\"}"))
	app.Handler().ServeHTTP(w, r)
	if w.Code != 503 || !strings.Contains(w.Body.String(), "START_UNAVAILABLE") {
		t.Fatalf("wrong classification: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "/nonexistent") {
		t.Fatal("worker path leaked")
	}
}

func TestMissingPlanHasSpecificSafeError(t *testing.T) {
	app, _ := setup(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/runs", bytes.NewBufferString("{\"plan\":\"absent\"}"))
	app.Handler().ServeHTTP(w, r)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "PLAN_INVALID") {
		t.Fatalf("wrong classification: %d %s", w.Code, w.Body.String())
	}
}

func TestSafeProductErrorDoesNotLeakRawDetails(t *testing.T) {
	w := httptest.NewRecorder()
	failure(w, errors.New("secret-token stack trace /private/provider SQL password"), 500)
	body := w.Body.String()
	for _, raw := range []string{"secret-token", "stack", "/private", "SQL", "password"} {
		if strings.Contains(body, raw) {
			t.Fatalf("raw details leaked: %s", raw)
		}
	}
	if !strings.Contains(body, "UNEXPECTED") {
		t.Fatal("missing safe code")
	}
}
