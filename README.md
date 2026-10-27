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
    IdempotencyKey:  "payout-456", // Optional, recommended
})
```

Withdrawals need an API key created with **Withdrawals** turned on, which requires two-factor authentication on your account.

`IdempotencyKey` (1-128 letters, digits, `-` or `_`) is sent as the `Idempotency-Key` header, so that a retry after a timeout or network error cannot send the money twice. For 24 hours, a repeat with the same key returns the first withdrawal instead of creating another; while the first request is still running, a repeat fails with a 409 `APIError`. Use one key per payout, such as your own payout id.

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

`VerifyIPN` confirms a received webhook (IPN) notification's transaction with the SagaPay API. Check the webhook signature first (see below); this network call is an extra check on top of it. The API credentials are sent in the request body for this endpoint, not in headers.

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

SagaPay sends webhook notifications to your specified `ipnUrl` when a transaction completes. Each one carries a signature over the exact raw request body:

```
X-Sagapay-Merchant-Signature: sha256=<hex HMAC-SHA256 of the raw body>
```

It is keyed with your **signing secret**. Copy it from the dashboard: **Webhooks → Signing secret**. It is not your API secret, and nobody else's secret can produce a valid signature for your notifications. `WebhookHandler` checks the signature on every notification, and a handler created with an empty secret rejects them all.

```go
package main

import (
    "log"
    "net/http"
    "os"

    "github.com/rootdigit/sagapay-go-sdk"
)

func main() {
    // Webhooks → Signing secret. Optional: also refuse notifications older than a day.
    webhookHandler := sagapay.NewWebhookHandler(os.Getenv("SAGAPAY_SIGNING_SECRET")).
        WithMaxAge(sagapay.DefaultMaxAge)

    http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
        // Verifies X-Sagapay-Merchant-Signature over the raw body, then parses it.
        payload, err := webhookHandler.HandleRequest(r)
        if err != nil {
            http.Error(w, "Invalid webhook", http.StatusUnauthorized)
            return
        }

        // Process each ID once. Delivery is at-least-once, and a replayed
        // request carries a valid signature too.
        if !markProcessed(payload.ID) { // e.g. INSERT ... ON CONFLICT DO NOTHING
            sagapay.SendSuccessResponse(w)
            return
        }

        switch payload.Type {
        case sagapay.IPNTypeDeposit:
            // Check what arrived before you credit it.
            creditOrder(payload.UDF, payload.Asset, payload.ContractAddress, payload.Amount)
        case sagapay.IPNTypeWithdrawal:
            markPayoutSent(payload.UDF, payload.TxHash)
        }

        // Answer 2xx within 20 seconds. If your own processing fails, answer
        // 5xx instead so SagaPay retries.
        sagapay.SendSuccessResponse(w)
    })

    log.Fatal(http.ListenAndServe(":8080", nil))
}
```

If you read the body yourself, pass it with the header value to `webhookHandler.ProcessWebhook(body, r.Header.Get(sagapay.SignatureHeader))`.

The old `X-Sagapay-Signature` header was keyed with a secret shared by every merchant. It is retired: SagaPay no longer sends it, and this SDK no longer reads it.

### Replay protection

A captured webhook still has a valid signature, so the signature alone does not stop someone resending it. Protect against that:

- **Process each `id` once.** Record the IDs you have handled (a unique column works) and answer duplicates with a 200 without acting on them again. This is the main defence and also covers normal retries.
- **Optionally check the age** with `WithMaxAge`, or `sagapay.IsFresh(payload, maxAge, time.Now())`. Keep the limit above the retry schedule (about 7.5 hours): retries reuse the original `timestamp`. A notification you resend from the dashboard also keeps it, so a resend older than your limit will be refused.
- **Check what you are crediting.** Match `Asset`, `ContractAddress`, `Amount` and `Address` against the order before you fulfil it.

## Webhook Payload Format

When SagaPay sends a webhook to your endpoint, it will include the following payload:

```json
{
  "id": "transaction-uuid",
  "type": "DEPOSIT|WITHDRAWAL",
  "status": "COMPLETED",
  "address": "0x123abc...",
  "networkType": "ERC20|BEP20|TRC20|POLYGON|SOLANA",
  "asset": "USDT",
  "contractAddress": "0x55d398326f99059fF775485246999027B3197955",
  "amount": "10.5",
  "udf": "your-optional-user-defined-field",
  "txHash": "0xabc123...",
  "timestamp": "2025-03-16T14:30:00Z"
}
```

Notes:

- `type` is uppercase (`DEPOSIT` or `WITHDRAWAL`), unlike the lowercase values used in API query parameters.
- `status` is currently always `COMPLETED` - IPNs are only sent for completed transactions.
- `asset` and `contractAddress` say what was received; `contractAddress` is `null` for the chain's native coin. `networkType` alone does not tell 100 TRX from 100 USDT.
- `udf` and `txHash` may be `null`.
- `timestamp` is set when the transaction completes and does not change on retries.
- SagaPay makes up to 6 delivery attempts, retrying after 1 min, 5 min, 15 min, 1 h and 6 h.

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