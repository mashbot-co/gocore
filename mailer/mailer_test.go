package mailer

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// captureRecord captures the body, headers, and reply behavior of one
// inbound SendGrid request.
type captureRecord struct {
	auth         string
	contentType  string
	body         []byte
	replyStatus  int
	replyBody    string
}

// captureServer stands in for SendGrid. It captures requests and responds
// with a configurable status code. Use Close() to shut it down.
func captureServer(t *testing.T, replyStatus int, replyBody string) (*httptest.Server, *atomic.Pointer[captureRecord]) {
	t.Helper()
	got := &atomic.Pointer[captureRecord]{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rec := &captureRecord{
			auth:        r.Header.Get("Authorization"),
			contentType: r.Header.Get("Content-Type"),
			body:        body,
			replyStatus: replyStatus,
			replyBody:   replyBody,
		}
		got.Store(rec)
		if replyStatus == 0 {
			replyStatus = 202
		}
		w.WriteHeader(replyStatus)
		if replyBody != "" {
			w.Write([]byte(replyBody))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

// installSendgrid wires a captureServer in as the mailer's HTTP target.
// Each test calls this so they don't interfere.
func installSendgrid(t *testing.T, srv *httptest.Server) {
	t.Helper()
	origEndpoint := sendgridEndpoint
	origClient := httpClient
	sendgridEndpoint = srv.URL
	httpClient = srv.Client()
	t.Cleanup(func() {
		sendgridEndpoint = origEndpoint
		httpClient = origClient
	})
}

// resetMailer clears all cached mailer state so each test starts fresh.
func resetMailer(t *testing.T) {
	t.Helper()
	resetConfig()
	t.Cleanup(resetConfig)
}

// --- loadConfig / AppURL ---

func TestAppURL_DefaultsToLocalhost(t *testing.T) {
	resetMailer(t)
	t.Setenv("SENDGRID_API_KEY", "key")
	t.Setenv("SENDGRID_FROM_EMAIL", "noreply@example.com")
	t.Setenv("APP_URL", "")

	if got := AppURL(); got != "http://localhost:5173" {
		t.Fatalf("expected http://localhost:5173 default, got %q", got)
	}
}

func TestAppURL_UsesConfigured(t *testing.T) {
	resetMailer(t)
	t.Setenv("SENDGRID_API_KEY", "key")
	t.Setenv("SENDGRID_FROM_EMAIL", "noreply@example.com")
	t.Setenv("APP_URL", "https://iro.studio/")

	if got := AppURL(); got != "https://iro.studio" {
		t.Fatalf("expected trailing slash stripped, got %q", got)
	}
}

func TestLoadConfig_FailsWithoutAPIKey(t *testing.T) {
	resetMailer(t)
	t.Setenv("SENDGRID_API_KEY", "")
	t.Setenv("SENDGRID_FROM_EMAIL", "noreply@example.com")

	loadConfig()
	if configErr == nil {
		t.Fatal("expected error when SENDGRID_API_KEY is unset")
	}
	if !strings.Contains(configErr.Error(), "SENDGRID_API_KEY") {
		t.Fatalf("expected SENDGRID_API_KEY in error, got %v", configErr)
	}
}

func TestLoadConfig_FailsWithoutFromEmail(t *testing.T) {
	resetMailer(t)
	t.Setenv("SENDGRID_API_KEY", "key")
	t.Setenv("SENDGRID_FROM_EMAIL", "")

	loadConfig()
	if configErr == nil {
		t.Fatal("expected error when SENDGRID_FROM_EMAIL is unset")
	}
	if !strings.Contains(configErr.Error(), "SENDGRID_FROM_EMAIL") {
		t.Fatalf("expected SENDGRID_FROM_EMAIL in error, got %v", configErr)
	}
}

func TestLoadConfig_ParsesDisplayNameFormat(t *testing.T) {
	resetMailer(t)
	t.Setenv("SENDGRID_API_KEY", "key")
	t.Setenv("SENDGRID_FROM_EMAIL", "Iro Studio <hello@iro.studio>")

	loadConfig()
	if fromName != "Iro Studio" {
		t.Errorf("expected fromName='Iro Studio', got %q", fromName)
	}
	if fromEmail != "hello@iro.studio" {
		t.Errorf("expected fromEmail='hello@iro.studio', got %q", fromEmail)
	}
}

func TestLoadConfig_ParsesPlainEmail(t *testing.T) {
	resetMailer(t)
	t.Setenv("SENDGRID_API_KEY", "key")
	t.Setenv("SENDGRID_FROM_EMAIL", "noreply@iro.studio")

	loadConfig()
	if fromName != "" {
		t.Errorf("expected empty fromName for plain email, got %q", fromName)
	}
	if fromEmail != "noreply@iro.studio" {
		t.Errorf("expected fromEmail='noreply@iro.studio', got %q", fromEmail)
	}
}

// --- Send happy path ---

// awaitSendgridRequest spins until the capture record is populated by the
// async send goroutine, or fails the test after a timeout.
func awaitSendgridRequest(t *testing.T, got *atomic.Pointer[captureRecord]) *captureRecord {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if rec := got.Load(); rec != nil {
			return rec
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for SendGrid request")
	return nil
}

func TestSend_PostsExpectedPayload(t *testing.T) {
	resetMailer(t)
	t.Setenv("SENDGRID_API_KEY", "test-key")
	t.Setenv("SENDGRID_FROM_EMAIL", "Iro <hello@iro.studio>")

	srv, captured := captureServer(t, 202, "")
	installSendgrid(t, srv)

	// Use the bundled invitation template so we don't need extra fixtures.
	Send("invitation.html", "You've been invited", map[string]any{
		"AppURL":   "https://iro.studio",
		"OrgName":  "Acme",
		"Inviter":  "Alice",
		"AcceptURL": "https://iro.studio/accept",
	}, []string{"user@example.com"})

	rec := awaitSendgridRequest(t, captured)
	if rec.auth != "Bearer test-key" {
		t.Errorf("expected Bearer test-key, got %q", rec.auth)
	}
	if rec.contentType != "application/json" {
		t.Errorf("expected application/json, got %q", rec.contentType)
	}

	var payload sgPayload
	if err := json.Unmarshal(rec.body, &payload); err != nil {
		t.Fatalf("expected valid JSON body: %v", err)
	}
	if payload.Subject != "You've been invited" {
		t.Errorf("subject: got %q", payload.Subject)
	}
	if payload.From.Email != "hello@iro.studio" || payload.From.Name != "Iro" {
		t.Errorf("from: got %+v", payload.From)
	}
	if len(payload.Personalizations) != 1 || len(payload.Personalizations[0].To) != 1 ||
		payload.Personalizations[0].To[0].Email != "user@example.com" {
		t.Errorf("to: got %+v", payload.Personalizations)
	}
	if len(payload.Content) != 1 || payload.Content[0].Type != "text/html" {
		t.Errorf("content: got %+v", payload.Content)
	}
}

func TestSend_NoOpWithEmptyRecipients(t *testing.T) {
	resetMailer(t)
	t.Setenv("SENDGRID_API_KEY", "k")
	t.Setenv("SENDGRID_FROM_EMAIL", "x@y")

	srv, captured := captureServer(t, 202, "")
	installSendgrid(t, srv)

	Send("invitation.html", "subject", map[string]any{}, []string{})

	// Give the async goroutine a moment; we should NOT have a captured request.
	time.Sleep(50 * time.Millisecond)
	if rec := captured.Load(); rec != nil {
		t.Fatalf("expected no SendGrid call for empty recipients, got %+v", rec)
	}
}

func TestSend_SkipsWhenConfigError(t *testing.T) {
	resetMailer(t)
	t.Setenv("SENDGRID_API_KEY", "") // forces configErr
	t.Setenv("SENDGRID_FROM_EMAIL", "x@y")

	srv, captured := captureServer(t, 202, "")
	installSendgrid(t, srv)

	Send("invitation.html", "subject", nil, []string{"a@b"})

	time.Sleep(50 * time.Millisecond)
	if rec := captured.Load(); rec != nil {
		t.Fatalf("expected no SendGrid call when config errors, got %+v", rec)
	}
}

func TestSend_SkipsOnTemplateError(t *testing.T) {
	resetMailer(t)
	t.Setenv("SENDGRID_API_KEY", "k")
	t.Setenv("SENDGRID_FROM_EMAIL", "x@y")

	srv, captured := captureServer(t, 202, "")
	installSendgrid(t, srv)

	// Template doesn't exist — renderTemplate returns an error; Send logs and returns.
	Send("nonexistent.html", "subject", nil, []string{"a@b"})

	time.Sleep(50 * time.Millisecond)
	if rec := captured.Load(); rec != nil {
		t.Fatalf("expected no SendGrid call when template errors, got %+v", rec)
	}
}

// --- send (private) error paths ---

func TestSend_LogsSendGridErrorStatus(t *testing.T) {
	resetMailer(t)
	t.Setenv("SENDGRID_API_KEY", "k")
	t.Setenv("SENDGRID_FROM_EMAIL", "x@y")

	srv, _ := captureServer(t, 401, "unauthorized")
	installSendgrid(t, srv)

	// Send is async. We can't easily observe the log output, but we CAN
	// exercise the non-2xx error path by calling the private `send` directly.
	loadConfig()
	err := send([]string{"a@b"}, "subject", "<p>body</p>")
	if err == nil {
		t.Fatal("expected error from non-2xx response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 in error, got %v", err)
	}
}

func TestSend_LogsTransportError(t *testing.T) {
	resetMailer(t)
	t.Setenv("SENDGRID_API_KEY", "k")
	t.Setenv("SENDGRID_FROM_EMAIL", "x@y")

	// Point at an unroutable endpoint.
	origEndpoint := sendgridEndpoint
	origClient := httpClient
	sendgridEndpoint = "http://127.0.0.1:1/sendgrid"
	httpClient = &http.Client{Timeout: 200 * time.Millisecond}
	t.Cleanup(func() {
		sendgridEndpoint = origEndpoint
		httpClient = origClient
	})

	loadConfig()
	err := send([]string{"a@b"}, "subject", "<p>body</p>")
	if err == nil {
		t.Fatal("expected transport error from unroutable endpoint")
	}
	if !strings.Contains(err.Error(), "send") {
		t.Fatalf("expected wrapped send error, got %v", err)
	}
}

func TestSend_RejectsInvalidEndpoint(t *testing.T) {
	resetMailer(t)
	t.Setenv("SENDGRID_API_KEY", "k")
	t.Setenv("SENDGRID_FROM_EMAIL", "x@y")

	// http.NewRequest rejects URLs with control characters in the scheme.
	origEndpoint := sendgridEndpoint
	sendgridEndpoint = "://invalid"
	t.Cleanup(func() { sendgridEndpoint = origEndpoint })

	loadConfig()
	err := send([]string{"a@b"}, "subject", "<p>body</p>")
	if err == nil {
		t.Fatal("expected error from invalid endpoint URL")
	}
	if !strings.Contains(err.Error(), "request") {
		t.Fatalf("expected wrapped request error, got %v", err)
	}
}

// Sanity: verify the JSON marshal error path is genuinely unreachable for our
// fixed payload type. We document this so future maintainers don't try to
// pad coverage by reaching into the json.Marshal call.
func TestSend_PayloadAlwaysMarshalable(t *testing.T) {
	payload := sgPayload{
		Personalizations: []sgPersonalization{{To: []sgEmail{{Email: "a@b"}}}},
		From:             sgEmail{Email: "x@y"},
		Subject:          "s",
		Content:          []sgContent{{Type: "text/html", Value: "<p>hi</p>"}},
	}
	if _, err := json.Marshal(payload); err != nil {
		t.Fatalf("expected payload to marshal cleanly, got %v", err)
	}
}

// Avoid unused-helper warnings.
var _ = fmt.Sprintf
