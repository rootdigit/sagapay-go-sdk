package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/rootdigit/sagapay-go-sdk"
)

func main() {
	// Your signing secret: dashboard → Webhooks → Signing secret. It is NOT
	// your API secret. Notifications whose X-Sagapay-Merchant-Signature does
	// not match it are rejected.
	signingSecret := os.Getenv("SAGAPAY_SIGNING_SECRET")
	if signingSecret == "" {
		log.Fatal("Set SAGAPAY_SIGNING_SECRET to your signing secret from the dashboard's Webhooks page")
	}

	// Optional: also refuse notifications older than a day.
	webhookHandler := sagapay.NewWebhookHandler(signingSecret).WithMaxAge(sagapay.DefaultMaxAge)

	// IDs already handled. Use a database table with a unique column in
	// production; this map is lost on restart.
	var mu sync.Mutex
	processed := map[string]bool{}

	http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Verifies the signature over the exact raw body, then parses it.
		payload, err := webhookHandler.HandleRequest(r)
		if err != nil {
			log.Printf("Rejected webhook: %v", err)
			http.Error(w, "Invalid webhook", http.StatusUnauthorized)
			return
		}

		log.Printf("Received webhook: ID=%s, Type=%s, Asset=%s, Amount=%s", payload.ID, payload.Type, payload.Asset, payload.Amount)

		// Delivery is at-least-once, and a replayed request carries a valid
		// signature too, so act on each ID only once.
		mu.Lock()
		seen := processed[payload.ID]
		processed[payload.ID] = true
		mu.Unlock()
		if seen {
			sagapay.SendSuccessResponse(w)
			return
		}

		// The status field is currently always COMPLETED - SagaPay only sends
		// IPNs for completed transactions.
		switch payload.Type {
		case sagapay.IPNTypeDeposit:
			// Check payload.Asset, payload.ContractAddress and payload.Amount
			// against the order before you mark it paid.
			log.Printf("Deposit %s completed: %s %s", payload.ID, payload.Amount, payload.Asset)
			// updateOrderStatus(payload.UDF, "paid")

		case sagapay.IPNTypeWithdrawal:
			log.Printf("Withdrawal %s completed: %s", payload.ID, payload.TxHash)
			// updateWithdrawalStatus(payload.UDF, "completed")
		}

		// Answer within 20 seconds. If your own processing fails, answer 5xx
		// instead so SagaPay retries.
		sagapay.SendSuccessResponse(w)
	})

	server := &http.Server{
		Addr:              ":8080",
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       20 * time.Second,
	}
	fmt.Println("Starting webhook server on :8080...")
	log.Fatal(server.ListenAndServe())
}
