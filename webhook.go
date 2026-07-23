package sagapay

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// WebhookHandler handles SagaPay webhook (IPN) notifications
type WebhookHandler struct {
	ipnSecret string
}

// NewWebhookHandler creates a new webhook handler.
//
// ipnSecret is the platform-issued IPN signing secret (an optional feature),
// NOT your API secret. If ipnSecret is empty, signature verification is
// skipped and webhook payloads are only parsed. Either way, Client.VerifyIPN
// is the primary check for confirming a notification is genuine.
func NewWebhookHandler(ipnSecret string) *WebhookHandler {
	return &WebhookHandler{
		ipnSecret: ipnSecret,
	}
}

// HandleRequest processes a webhook notification from an HTTP request
func (h *WebhookHandler) HandleRequest(r *http.Request) (*WebhookPayload, error) {
	// Get the signature from the headers
	signature := r.Header.Get("X-Sagapay-Signature")
	if h.ipnSecret != "" && signature == "" {
		return nil, errors.New("missing SagaPay signature in headers")
	}

	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	return h.ProcessWebhook(body, signature)
}

// ProcessWebhook processes a webhook notification from raw body and signature.
//
// If the handler was created without an IPN secret, signature verification is
// skipped and the payload is only parsed; use Client.VerifyIPN as the primary
// check before trusting the notification.
func (h *WebhookHandler) ProcessWebhook(body []byte, signature string) (*WebhookPayload, error) {
	// Verify the signature (skipped when no IPN secret is configured)
	if h.ipnSecret != "" && !h.VerifySignature(body, signature) {
		return nil, errors.New("invalid webhook signature")
	}

	// Parse the webhook payload
	var payload WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	return &payload, nil
}

// VerifySignature verifies the HMAC-SHA256 signature of a webhook payload.
// The signature is the value of the X-Sagapay-Signature header, either in
// the "sha256=<hex>" form sent by SagaPay or as bare hex.
func (h *WebhookHandler) VerifySignature(payload []byte, signature string) bool {
	signature = strings.TrimPrefix(signature, "sha256=")

	// Calculate the HMAC-SHA256 of the raw body
	mac := hmac.New(sha256.New, []byte(h.ipnSecret))
	mac.Write(payload)
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	// Compare with the provided signature (constant time)
	return hmac.Equal([]byte(expectedSignature), []byte(signature))
}

// SendSuccessResponse sends a success response for a webhook
func SendSuccessResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"received": true})
}

// SendErrorResponse sends an error response for a webhook
func SendErrorResponse(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	// Still return 200 OK to prevent retries
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"received": false,
		"error":    err.Error(),
	})
}
