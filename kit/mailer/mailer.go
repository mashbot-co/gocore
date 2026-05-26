package mailer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

var (
	fromEmail string
	fromName  string
	apiKey    string
	appURL    string
	configErr error
	once      sync.Once

	// httpClient is the HTTP client used to call SendGrid. Replaceable for tests.
	httpClient = http.DefaultClient
	// sendgridEndpoint is the SendGrid v3 endpoint. Replaceable for tests.
	sendgridEndpoint = "https://api.sendgrid.com/v3/mail/send"
)

// resetConfig clears the cached config (for tests). Not exported.
func resetConfig() {
	fromEmail = ""
	fromName = ""
	apiKey = ""
	appURL = ""
	configErr = nil
	once = sync.Once{}
}

func loadConfig() {
	once.Do(func() {
		apiKey = os.Getenv("SENDGRID_API_KEY")
		if apiKey == "" {
			configErr = fmt.Errorf("mailer: SENDGRID_API_KEY not set")
			return
		}

		raw := os.Getenv("SENDGRID_FROM_EMAIL")
		if raw == "" {
			configErr = fmt.Errorf("mailer: SENDGRID_FROM_EMAIL not set")
			return
		}

		// Parse "Display Name <email@domain>" format
		if idx := strings.Index(raw, "<"); idx != -1 {
			fromName = strings.TrimSpace(raw[:idx])
			fromEmail = strings.Trim(raw[idx:], "<> ")
		} else {
			fromEmail = strings.TrimSpace(raw)
		}

		appURL = strings.TrimRight(os.Getenv("APP_URL"), "/")
		if appURL == "" {
			appURL = "http://localhost:5173"
		}
	})
}

// AppURL returns the configured application URL.
func AppURL() string {
	loadConfig()
	return appURL
}

// SendGrid API types

type sgEmail struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type sgPersonalization struct {
	To []sgEmail `json:"to"`
}

type sgContent struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type sgPayload struct {
	Personalizations []sgPersonalization `json:"personalizations"`
	From             sgEmail             `json:"from"`
	Subject          string              `json:"subject"`
	Content          []sgContent         `json:"content"`
}

// Send renders the named template with the given data and sends the email
// asynchronously via SendGrid. Errors are logged but do not block the caller.
//
// Usage:
//
//	mailer.Send("invitation.html", "You've been invited to Acme", data, []string{"user@example.com"})
func Send(templateName string, subject string, data any, to []string) {
	loadConfig()
	if configErr != nil {
		log.Printf("[mailer] config error, skipping email: %v", configErr)
		return
	}

	if len(to) == 0 {
		return
	}

	body, err := renderTemplate(templateName, data)
	if err != nil {
		log.Printf("[mailer] template error (%s): %v", templateName, err)
		return
	}

	go func() {
		if err := send(to, subject, body); err != nil {
			log.Printf("[mailer] error: %v", err)
		}
	}()
}

// send posts an email via SendGrid v3 API.
func send(to []string, subject, htmlBody string) error {
	recipients := make([]sgEmail, len(to))
	for i, addr := range to {
		recipients[i] = sgEmail{Email: addr}
	}

	payload := sgPayload{
		Personalizations: []sgPersonalization{{To: recipients}},
		From:             sgEmail{Email: fromEmail, Name: fromName},
		Subject:          subject,
		Content:          []sgContent{{Type: "text/html", Value: htmlBody}},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("mailer: marshal: %w", err)
	}

	req, err := http.NewRequest("POST", sendgridEndpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("mailer: request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("mailer: send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var respBody bytes.Buffer
		respBody.ReadFrom(resp.Body)
		return fmt.Errorf("mailer: sendgrid returned %d: %s", resp.StatusCode, respBody.String())
	}

	return nil
}
