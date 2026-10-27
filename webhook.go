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
	"time"
)

const (
	// SignatureHeader is the header that carries an IPN's signature.
	SignatureHeader = "X-Sagapay-Merchant-Signature"

	// DefaultMaxAge is a sensible limit for IsFresh and WithMaxAge. It is
	// well above the IPN retry schedule (about 7.5 hours).
	DefaultMaxAge = 24 * time.Hour

	// maxWebhookBody caps how much of a request HandleRequest reads.
	// Notifications are a few hundred bytes.
	maxWebhookBody = 1 << 20

	// maxClockSkew is how far ahead of this machine's clock a timestamp may
	// be before it is treated as invalid rather than as clock drift.
	maxClockSkew = 5 * time.Minute
)

// WebhookHandler verifies and parses SagaPay webhook (IPN) notifications.
type WebhookHandler struct {
	signingSecret string
	maxAge        time.Duration
}

// NewWebhookHandler creates a new webhook handler.
//
// signingSecret is your signing secret: copy it from the dashboard
// (Webhooks → Signing secret). It is NOT your API secret. Every notification
// is checked against the X-Sagapay-Merchant-Signature header; a handler with
// an empty secret rejects every notification.
func NewWebhookHandler(signingSecret string) *WebhookHandler {
	return &WebhookHandler{
		signingSecret: signingSecret,
	}
}

// WithMaxAge makes the handler also reject notifications whose timestamp is
// older than maxAge (see IsFresh). Keep it above the ~7.5 hour retry
// schedule; DefaultMaxAge is a sensible value.
func (h *WebhookHandler) WithMaxAge(maxAge time.Duration) *WebhookHandler {
	h.maxAge = maxAge
	return h
}

// HandleRequest verifies and parses a webhook notification from an HTTP request.
func (h *WebhookHandler) HandleRequest(r *http.Request) (*WebhookPayload, error) {
	// A repeated header is never valid; do not guess which one to trust.
	signatures := r.Header.Values(SignatureHeader)
	if len(signatures) != 1 || signatures[0] == "" {
		return nil, errors.New("missing SagaPay signature in headers")
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBody+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	if len(body) > maxWebhookBody {
		return nil, errors.New("webhook body too large")
	}

	return h.ProcessWebhook(body, signatures[0])
}

// ProcessWebhook verifies and parses a webhook notification from its exact
// raw body and the value of its X-Sagapay-Merchant-Signature header.
func (h *WebhookHandler) ProcessWebhook(body []byte, signature string) (*WebhookPayload, error) {
	if h.signingSecret == "" {
		return nil, errors.New("no signing secret configured: copy it from the dashboard's Webhooks page")
	}
	if !h.VerifySignature(body, signature) {
		return nil, errors.New("invalid webhook signature")
	}

	var payload WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	if h.maxAge > 0 && !IsFresh(&payload, h.maxAge, time.Now()) {
		return nil, errors.New("webhook timestamp is outside the allowed age")
	}

	return &payload, nil
}

// VerifySignature reports whether signature is the HMAC-SHA256 of payload
// keyed with the handler's signing secret. The signature is the value of the
// X-Sagapay-Merchant-Signature header, either in the "sha256=<hex>" form sent
// by SagaPay or as bare hex. It is false when no signing secret is configured.
func (h *WebhookHandler) VerifySignature(payload []byte, signature string) bool {
	if h.signingSecret == "" {
		return false
	}
	signature = strings.TrimSpace(signature)
	if len(signature) >= 7 && strings.EqualFold(signature[:7], "sha256=") {
		signature = signature[7:]
	}
	given, err := hex.DecodeString(signature)
	if err != nil || len(given) != sha256.Size {
		return false
	}

	mac := hmac.New(sha256.New, []byte(h.signingSecret))
	mac.Write(payload)

	// Constant-time comparison
	return hmac.Equal(mac.Sum(nil), given)
}

// IsFresh reports whether a notification's timestamp is recent enough to act
// on, as a guard against an old, validly signed notification being replayed.
//
// The timestamp is set when the transaction completes and stays the same on
// every retry, so keep maxAge longer than the retry schedule (about 7.5
// hours). A notification resent from the dashboard also keeps its original
// timestamp, and is rejected once it is older than maxAge.
//
// This is a secondary check. The main defence against replays is to process
// each notification ID only once.
func IsFresh(payload *WebhookPayload, maxAge time.Duration, now time.Time) bool {
	if payload == nil || payload.Timestamp.IsZero() {
		return false
	}
	return !payload.Timestamp.After(now.Add(maxClockSkew)) && now.Sub(payload.Timestamp) <= maxAge
}

// SendSuccessResponse sends a success response for a webhook
func SendSuccessResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"received": true})
}

// SendErrorResponse sends an error response for a webhook. It answers 200 so
// SagaPay does not retry; use it for notifications you reject. If your own
// processing failed and you want the notification again, answer with a 5xx
// instead.
func SendErrorResponse(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"received": false,
		"error":    err.Error(),
	})
}
