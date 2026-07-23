package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/rootdigit/sagapay-go-sdk"
)

func main() {
	// Initialize the SagaPay client, used to confirm notifications via verify-ipn
	client, err := sagapay.NewClient(sagapay.Config{
		APIKey:    "your-api-key",
		APISecret: "your-api-secret",
	})
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// The platform-issued IPN signing secret (optional feature). This is NOT
	// your API secret. Leave it empty if you have not been issued one; the
	// handler then skips signature verification and the VerifyIPN call below
	// is the primary check.
	ipnSecret := ""

	// Create a webhook handler
	webhookHandler := sagapay.NewWebhookHandler(ipnSecret)

	// Set up a handler for webhook notifications
	http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		// Only accept POST requests
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Process the webhook
		payload, err := webhookHandler.HandleRequest(r)
		if err != nil {
			log.Printf("Error processing webhook: %v", err)
			sagapay.SendErrorResponse(w, err)
			return
		}

		// Log the webhook
		log.Printf("Received webhook: ID=%s, Type=%s, Status=%s", payload.ID, payload.Type, payload.Status)

		// Confirm the notification with SagaPay before trusting it. This is
		// the primary verification step. Note that SagaPay delivers IPNs
		// at least once, so the same notification may arrive more than once;
		// make your business logic idempotent.
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		verification, err := client.VerifyIPN(ctx, sagapay.VerifyIPNParams{
			TxnHash: payload.TxHash,
			Type:    payload.Type,
			Amount:  payload.Amount,
			Address: payload.Address,
		})
		if err != nil {
			log.Printf("Error verifying IPN: %v", err)
			sagapay.SendErrorResponse(w, err)
			return
		}
		if !verification.Verified {
			log.Printf("IPN could not be verified, ignoring: ID=%s", payload.ID)
			sagapay.SendErrorResponse(w, fmt.Errorf("IPN not verified"))
			return
		}

		// Handle the notification by type. The status field is currently
		// always COMPLETED - SagaPay only sends IPNs for completed transactions.
		switch payload.Type {
		case sagapay.IPNTypeDeposit:
			log.Printf("Deposit %s completed: %s", payload.ID, payload.Amount)

			// Your business logic here...
			// For example, update order status in your database
			// updateOrderStatus(payload.UDF, "paid")

		case sagapay.IPNTypeWithdrawal:
			log.Printf("Withdrawal %s completed: %s", payload.ID, payload.Amount)

			// Your business logic here...
			// updateWithdrawalStatus(payload.UDF, "completed")
		}

		// Send a success response
		sagapay.SendSuccessResponse(w)
	})

	// Start the server
	fmt.Println("Starting webhook server on :8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
