package sagapay

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
	secret := "test-signing-secret"
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

func TestProcessWebhookRejectsWithoutSecret(t *testing.T) {
	handler := NewWebhookHandler("")

	body := []byte(`{"id":"transaction-uuid","type":"WITHDRAWAL","status":"COMPLETED","address":"0x123abc","networkType":"ERC20","amount":"10.5","txHash":"0xabc123","timestamp":"2025-03-16T14:30:00Z"}`)

	if _, err := handler.ProcessWebhook(body, "sha256="+signBody("", body)); err == nil {
		t.Fatal("expected a handler without a signing secret to reject the webhook")
	}
	if handler.VerifySignature(body, "sha256="+signBody("", body)) {
		t.Fatal("expected VerifySignature to be false without a signing secret")
	}
}

func TestVerifySignatureMatchesSagaPaySigner(t *testing.T) {
	// Produced by the SagaPay signer (core-api utils/ipnSignature.js) for a
	// test merchant; keeps this SDK in step with what the platform sends.
	handler := NewWebhookHandler("f59949bf79a73e05a41f10d2333a41b80644430f5220f468171392df355663a7")
	body := []byte(`{"id":"dep-1","type":"DEPOSIT","status":"COMPLETED","address":"0xabc","networkType":"BEP20","asset":"USDT","contractAddress":"0x55d398326f99059fF775485246999027B3197955","amount":"100.5","udf":null,"txHash":"0xdef","timestamp":"2026-10-27T00:00:00.000Z"}`)
	header := "sha256=a9841956a4b8ccf048f953081db6892e69da181b5792dfcf8604ea4795f6a469"

	payload, err := handler.ProcessWebhook(body, header)
	if err != nil {
		t.Fatalf("expected the SagaPay signature to verify, got: %v", err)
	}
	if payload.Asset != "USDT" || payload.ContractAddress != "0x55d398326f99059fF775485246999027B3197955" {
		t.Errorf("expected asset and contractAddress to parse, got %q %q", payload.Asset, payload.ContractAddress)
	}
	if handler.VerifySignature([]byte(strings.Replace(string(body), "100.5", "100.6", 1)), header) {
		t.Error("expected a tampered body to be rejected")
	}
}

func TestVerifySignatureFormats(t *testing.T) {
	secret := "test-signing-secret"
	handler := NewWebhookHandler(secret)
	body := []byte(`{"id":"x"}`)
	digest := signBody(secret, body)

	if !handler.VerifySignature(body, "SHA256="+strings.ToUpper(digest)) {
		t.Error("expected upper-case hex and prefix to be accepted")
	}
	for _, bad := range []string{"", "sha256=", digest[:62], digest + "00", "zz" + digest[2:]} {
		if handler.VerifySignature(body, bad) {
			t.Errorf("expected %q to be rejected", bad)
		}
	}
}

func newWebhookRequest(body []byte, headers map[string][]string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	for name, values := range headers {
		for _, v := range values {
			r.Header.Add(name, v)
		}
	}
	return r
}

func TestHandleRequestReadsOnlyTheMerchantHeader(t *testing.T) {
	secret := "test-signing-secret"
	handler := NewWebhookHandler(secret)
	body := []byte(`{"id":"transaction-uuid","type":"DEPOSIT","status":"COMPLETED","address":"0x123abc","networkType":"BEP20","amount":"10.5","timestamp":"2025-03-16T14:30:00Z"}`)
	sig := "sha256=" + signBody(secret, body)

	if _, err := handler.HandleRequest(newWebhookRequest(body, map[string][]string{SignatureHeader: {sig}})); err != nil {
		t.Fatalf("expected X-Sagapay-Merchant-Signature to verify, got: %v", err)
	}
	if _, err := handler.HandleRequest(newWebhookRequest(body, map[string][]string{"X-Sagapay-Signature": {sig}})); err == nil {
		t.Error("expected the retired X-Sagapay-Signature header to be ignored")
	}
	if _, err := handler.HandleRequest(newWebhookRequest(body, map[string][]string{SignatureHeader: {sig, sig}})); err == nil {
		t.Error("expected a repeated signature header to be rejected")
	}
}

func TestHandleRequestRejectsOversizedBody(t *testing.T) {
	secret := "test-signing-secret"
	handler := NewWebhookHandler(secret)
	body := bytes.Repeat([]byte("a"), maxWebhookBody+1)
	r := newWebhookRequest(body, map[string][]string{SignatureHeader: {"sha256=" + signBody(secret, body)}})
	if _, err := handler.HandleRequest(r); err == nil {
		t.Error("expected an oversized body to be rejected")
	}
}

func TestIsFresh(t *testing.T) {
	sent := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	payload := &WebhookPayload{Timestamp: sent}

	cases := []struct {
		now    time.Time
		maxAge time.Duration
		want   bool
	}{
		{sent.Add(time.Minute), time.Hour, true},
		{sent.Add(time.Hour + time.Second), time.Hour, false},
		{sent.Add(8 * time.Hour), DefaultMaxAge, true},
		{sent.Add(25 * time.Hour), DefaultMaxAge, false},
		{sent.Add(-time.Minute), time.Hour, true},
		{sent.Add(-10 * time.Minute), time.Hour, false},
	}
	for _, c := range cases {
		if got := IsFresh(payload, c.maxAge, c.now); got != c.want {
			t.Errorf("IsFresh(now=%v, maxAge=%v) = %v, want %v", c.now, c.maxAge, got, c.want)
		}
	}
	if IsFresh(&WebhookPayload{}, time.Hour, sent) {
		t.Error("expected a missing timestamp to be rejected")
	}
}

func TestWithMaxAgeRejectsStaleWebhook(t *testing.T) {
	secret := "test-signing-secret"
	body := []byte(`{"id":"transaction-uuid","type":"DEPOSIT","status":"COMPLETED","address":"0x123abc","networkType":"BEP20","amount":"10.5","timestamp":"2025-03-16T14:30:00Z"}`)
	sig := "sha256=" + signBody(secret, body)

	if _, err := NewWebhookHandler(secret).WithMaxAge(DefaultMaxAge).ProcessWebhook(body, sig); err == nil {
		t.Error("expected a webhook from 2025 to be rejected as stale")
	}
	if _, err := NewWebhookHandler(secret).ProcessWebhook(body, sig); err != nil {
		t.Errorf("expected no age check without WithMaxAge, got: %v", err)
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
