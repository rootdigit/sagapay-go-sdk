package sagapay

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewClient(Config{APIKey: "key", APISecret: "secret", BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

var testWithdrawal = CreateWithdrawalParams{
	NetworkType:     NetworkTypeBEP20,
	ContractAddress: "0",
	Address:         "0x742d35Cc6634C0532925a3b844Bc454e4438f44e",
	Amount:          "0.5",
	IPNUrl:          "https://example.com/ipn",
}

func TestCreateWithdrawalSendsIdempotencyKey(t *testing.T) {
	var got []string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Values("Idempotency-Key")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"w-1","status":"PENDING","fee":"0"}`))
	})

	params := testWithdrawal
	params.IdempotencyKey = "payout-123"
	if _, err := client.CreateWithdrawal(context.Background(), params); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "payout-123" {
		t.Errorf("expected one Idempotency-Key header \"payout-123\", got %q", got)
	}

	if _, err := client.CreateWithdrawal(context.Background(), testWithdrawal); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected no Idempotency-Key header by default, got %q", got)
	}
}

func TestRequestsDoNotFollowRedirects(t *testing.T) {
	var paths []string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/create-withdrawal" {
			http.Redirect(w, r, "/elsewhere", http.StatusTemporaryRedirect)
			return
		}
		w.Write([]byte(`{"id":"w-1","status":"PENDING","fee":"0"}`))
	})

	_, err := client.CreateWithdrawal(context.Background(), testWithdrawal)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected an APIError for the redirect, got %v", err)
	}
	if len(paths) != 1 {
		t.Errorf("expected the redirect not to be followed, saw requests to %q", paths)
	}
}
