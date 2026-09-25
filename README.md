# SagaPay Go SDK

Go SDK for [SagaPay](https://sagapay.io) - the world's first free, non-custodial blockchain payment gateway service provider. This SDK enables Go developers to seamlessly integrate cryptocurrency payments without holding customer funds. With enterprise-grade security and zero transaction fees, SagaPay empowers merchants to accept crypto payments across multiple blockchains while maintaining full control of their digital assets.

## Installation

```bash
go get github.com/rootdigit/sagapay-go-sdk
```

## Quick Start

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/rootdigit/sagapay-go-sdk"
)

func main() {
	// Initialize the SagaPay client
	client, err := sagapay.NewClient(sagapay.Config{
		APIKey:    "your-api-key",
		APISecret: "your-api-secret",
	})
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// Create a deposit address
	depositResponse, err := client.CreateDeposit(context.Background(), sagapay.CreateDepositParams{
		NetworkType:     sagapay.NetworkTypeBEP20,
		ContractAddress: "0", // Use '0' for native tokens (BNB)
		Amount:          "1.5",
		IPNUrl:          "https://yourwebsite.com/webhook",
		UDF:             "order-123",
		Type:            sagapay.AddressTypeTemporary,
	})
	if err != nil {
		log.Fatalf("Failed to create deposit: %v", err)
	}

	fmt.Printf("Deposit address created: %s\n", depositResponse.Address)
}
```

## Features

- Deposit address generation
- Withdrawal processing
- Transaction status checking
- Wallet balance fetching
- Multi-chain support (ERC20, BEP20, TRC20, POLYGON, SOLANA)
- Webhook notifications (IPN)
- Custom UDF field support
- Non-custodial architecture
- Context support for proper cancellation handling

## API Reference

### Create Deposit

```go
transferBalance := true
depositResponse, err := client.CreateDeposit(ctx, sagapay.CreateDepositParams{
    NetworkType:     sagapay.NetworkTypeBEP20,     // Required: Blockchain network type
    ContractAddress: "0",                          // Required: Contract address or '0' for native coins
    Amount:          "1.5",                        // Required: Expected deposit amount
    IPNUrl:          "https://example.com/webhook", // Required: URL for notifications
    UDF:             "order-123",                  // Optional: User-defined field
    Type:            sagapay.AddressTypeTemporary, // Optional: TEMPORARY or PERMANENT
    TransferBalance: &transferBalance,             // Optional: defaults to true when omitted
})
```

### Create Withdrawal

```go
withdrawalResponse, err := client.CreateWithdrawal(ctx, sagapay.CreateWithdrawalParams{
    NetworkType:     sagapay.NetworkTypeERC20,
    ContractAddress: "0xdAC17F958D2ee523a2206206994597C13D831ec7", // USDT on Ethereum
    Address:         "0x742d35Cc6634C0532925a3b844Bc454e4438f44e",
    Amount:          "10.5",
    IPNUrl:          "https://example.com/webhook",
    UDF:             "withdrawal-456",
})
```

### Check Transaction Status

```go
// By address
statusResponse, err := client.CheckTransactionStatus(
    ctx,
    sagapay.TransactionTypeDeposit,
    sagapay.CheckTransactionStatusOptions{
        Address: "0x742d35Cc6634C0532925a3b844Bc454e4438f44e",
    },
)

// By transaction ID
statusResponse, err := client.CheckTransactionStatus(
    ctx,
    sagapay.TransactionTypeDeposit,
    sagapay.CheckTransactionStatusOptions{
        ID: "deposit-uuid",
    },
)
```

### Fetch Wallet Balance

```go
balanceResponse, err := client.FetchWalletBalance(
    ctx,
    "0x742d35Cc6634C0532925a3b844Bc454e4438f44e", // Address
    sagapay.NetworkTypeERC20,                      // Network type
    "0xdAC17F958D2ee523a2206206994597C13D831ec7", // Contract address
)
```

### Verify IPN

`VerifyIPN` confirms a received webhook (IPN) notification against the SagaPay API and is the primary way to check that a notification is genuine. The API credentials are sent in the request body for this endpoint, not in headers.

```go
verification, err := client.VerifyIPN(ctx, sagapay.VerifyIPNParams{
    TxnHash: payload.TxHash,        // Transaction hash from the notification
    Type:    sagapay.IPNTypeDeposit, // IPNTypeDeposit or IPNTypeWithdrawal (uppercase)
    Amount:  payload.Amount,
    Address: payload.Address,
})
if err != nil {
    log.Fatalf("Failed to verify IPN: %v", err)
}
if verification.Verified {
    // The notification is genuine
}
```

## Handling Webhooks (IPN)

SagaPay sends webhook notifications to your specified `ipnUrl` when a transaction completes. Delivery is at-least-once, so the same notification may arrive more than once - make your handling idempotent.

Use the `WebhookHandler` to parse notifications and `client.VerifyIPN` to confirm them:

```go
package main

import (
    "context"
    "errors"
    "log"
    "net/http"
    "time"

    "github.com/rootdigit/sagapay-go-sdk"
)

func main() {
    client, err := sagapay.NewClient(sagapay.Config{
        APIKey:    "your-api-key",
        APISecret: "your-api-secret",
    })
    if err != nil {
        log.Fatalf("Failed to create client: %v", err)
    }

    // Create a webhook handler with your platform-issued IPN secret (an
    // optional feature) - this is NOT your API secret. Pass "" if you have
    // not been issued one; signature verification is then skipped and the
    // VerifyIPN call below is the primary check.
    webhookHandler := sagapay.NewWebhookHandler("")

    // Set up a handler for webhook notifications
    http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
        // Parse the webhook (the signature is verified if an IPN secret is set)
        payload, err := webhookHandler.HandleRequest(r)
        if err != nil {
            log.Printf("Error processing webhook: %v", err)
            sagapay.SendErrorResponse(w, err)
            return
        }

        // Confirm the notification with SagaPay - the primary check
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
            sagapay.SendErrorResponse(w, errors.New("IPN not verified"))
            return
        }

        // Handle the notification by type (status is currently always COMPLETED)
        switch payload.Type {
        case sagapay.IPNTypeDeposit:
            // Payment successful, update your database
            log.Printf("Deposit %s completed", payload.ID)
        case sagapay.IPNTypeWithdrawal:
            log.Printf("Withdrawal %s completed", payload.ID)
        }

        // Send a success response
        sagapay.SendSuccessResponse(w)
    })

    // Start the server
    log.Fatal(http.ListenAndServe(":8080", nil))
}
```

## Webhook Payload Format

When SagaPay sends a webhook to your endpoint, it will include the following payload:

```json
{
  "id": "transaction-uuid",
  "type": "DEPOSIT|WITHDRAWAL",
  "status": "COMPLETED",
  "address": "0x123abc...",
  "networkType": "ERC20|BEP20|TRC20|POLYGON|SOLANA",
  "amount": "10.5",
  "udf": "your-optional-user-defined-field",
  "txHash": "0xabc123...",
  "timestamp": "2025-03-16T14:30:00Z"
}
```

Notes:

- `type` is uppercase (`DEPOSIT` or `WITHDRAWAL`), unlike the lowercase values used in API query parameters.
- `status` is currently always `COMPLETED` - IPNs are only sent for completed transactions.
- `udf` and `txHash` may be `null`.
- Delivery is at-least-once: process notifications idempotently.

### Signature Header

If you have been issued a platform IPN secret (an optional feature), each webhook request is signed with the header:

```
X-Sagapay-Signature: sha256=<hex HMAC-SHA256 of the exact raw request body>
```

The HMAC is keyed with the platform-issued IPN secret, NOT your API secret. `WebhookHandler` verifies this header automatically when constructed with the secret. Whether or not a signature is present, `client.VerifyIPN` is the primary way to confirm a notification is genuine.

## Error Handling

The SDK includes comprehensive error handling:

```go
depositResponse, err := client.CreateDeposit(ctx, params)
if err != nil {
    // Check if it's an API error
    if apiErr, ok := err.(*sagapay.APIError); ok {
        fmt.Printf("API Error (HTTP %d): %s\n", apiErr.Code, apiErr.ErrMessage)
        return
    }
    
    // Handle other errors
    fmt.Printf("Error: %v\n", err)
    return
}
```

## License

This SDK is released under the MIT License.

## Support

For questions or support, please contact support@sagapay.io or visit [https://sagapay.io](https://sagapay.io).