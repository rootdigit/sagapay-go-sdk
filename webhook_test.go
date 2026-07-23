package sagapay

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"
)

// signBody computes the hex HMAC-SHA256 of body keyed with secret
func signBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	secret := "test-ipn-secret"
	handler := NewWebhookHandler(secret)

	body := []byte(`{"id":"transaction-uuid","type":"DEPOSIT","status":"COMPLETED","address":"0x123abc","networkType":"BEP20","amount":"10.5","udf":"order-123","txHash":"0xabc123","timestamp":"2025-03-16T14:30:00Z"}`)
	signature := signBody(secret, body)

	if !handler.VerifySignature(body, "sha256="+signature) {
		t.Error("expected signature with sha256= prefix to be accepted")
	}

	if !handler.VerifySignature(body, signature) {
		t.Error("expected bare hex signature to be accepted")
	}

	tampered := []byte(`{"id":"transaction-uuid","type":"DEPOSIT","status":"COMPLETED","address":"0x123abc","networkType":"BEP20","amount":"999.9","udf":"order-123","txHash":"0xabc123","timestamp":"2025-03-16T14:30:00Z"}`)
	if handler.VerifySignature(tampered, "sha256="+signature) {
		t.Error("expected signature over tampered body to be rejected")
	}
}

func TestProcessWebhookSkipsVerificationWithoutSecret(t *testing.T) {
	handler := NewWebhookHandler("")

	body := []byte(`{"id":"transaction-uuid","type":"WITHDRAWAL","status":"COMPLETED","address":"0x123abc","networkType":"ERC20","amount":"10.5","txHash":"0xabc123","timestamp":"2025-03-16T14:30:00Z"}`)

	payload, err := handler.ProcessWebhook(body, "")
	if err != nil {
		t.Fatalf("expected verification to be skipped without an IPN secret, got error: %v", err)
	}
	if payload.Type != IPNTypeWithdrawal {
		t.Errorf("expected type %q, got %q", IPNTypeWithdrawal, payload.Type)
	}
}

func TestWebhookPayloadUnmarshal(t *testing.T) {
	body := []byte(`{
		"id": "transaction-uuid",
		"type": "DEPOSIT",
		"status": "COMPLETED",
		"address": "0x123abc",
		"networkType": "BEP20",
		"amount": "10.5",
		"udf": "order-123",
		"txHash": "0xabc123",
		"timestamp": "2025-03-16T14:30:00Z"
	}`)

	var payload WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("failed to unmarshal webhook payload: %v", err)
	}

	if payload.ID != "transaction-uuid" {
		t.Errorf("expected id %q, got %q", "transaction-uuid", payload.ID)
	}
	if payload.Type != IPNTypeDeposit {
		t.Errorf("expected type %q, got %q", IPNTypeDeposit, payload.Type)
	}
	if payload.Status != TransactionStatusCompleted {
		t.Errorf("expected status %q, got %q", TransactionStatusCompleted, payload.Status)
	}
	if payload.Address != "0x123abc" {
		t.Errorf("expected address %q, got %q", "0x123abc", payload.Address)
	}
	if payload.NetworkType != NetworkTypeBEP20 {
		t.Errorf("expected networkType %q, got %q", NetworkTypeBEP20, payload.NetworkType)
	}
	if payload.Amount != "10.5" {
		t.Errorf("expected amount %q, got %q", "10.5", payload.Amount)
	}
	if payload.UDF != "order-123" {
		t.Errorf("expected udf %q, got %q", "order-123", payload.UDF)
	}
	if payload.TxHash != "0xabc123" {
		t.Errorf("expected txHash %q, got %q", "0xabc123", payload.TxHash)
	}

	expectedTimestamp := time.Date(2025, 3, 16, 14, 30, 0, 0, time.UTC)
	if !payload.Timestamp.Equal(expectedTimestamp) {
		t.Errorf("expected timestamp %v, got %v", expectedTimestamp, payload.Timestamp)
	}
}
