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

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Example 1: Create a deposit address
	fmt.Println("Creating deposit address...")
	depositResponse, err := client.CreateDeposit(ctx, sagapay.CreateDepositParams{
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

	fmt.Println("✓ Deposit address created:")
	fmt.Printf("  ID: %s\n", depositResponse.ID)
	fmt.Printf("  Address: %s\n", depositResponse.Address)
	if depositResponse.ExpiresAt != nil {
		fmt.Printf("  Expires At: %s\n", depositResponse.ExpiresAt.Format(time.RFC3339))
	} else {
		fmt.Println("  Expires At: never (PERMANENT address)")
	}
	fmt.Printf("  Status: %s\n\n", depositResponse.Status)

	// Example 2: Create a withdrawal
	fmt.Println("Creating withdrawal...")
	withdrawalResponse, err := client.CreateWithdrawal(ctx, sagapay.CreateWithdrawalParams{
		NetworkType:     sagapay.NetworkTypeERC20,
		ContractAddress: "0xdAC17F958D2ee523a2206206994597C13D831ec7", // USDT on Ethereum
		Address:         "0x742d35Cc6634C0532925a3b844Bc454e4438f44e",
		Amount:          "10.5",
		IPNUrl:          "https://yourwebsite.com/webhook",
		UDF:             "withdrawal-456",
	})
	if err != nil {
		log.Fatalf("Failed to create withdrawal: %v", err)
	}

	fmt.Println("✓ Withdrawal created:")
	fmt.Printf("  ID: %s\n", withdrawalResponse.ID)
	fmt.Printf("  Status: %s\n", withdrawalResponse.Status)
	fmt.Printf("  Fee: %s\n\n", withdrawalResponse.Fee)

	// Example 3: Check transaction status
	fmt.Println("Checking transaction status...")
	address := "0x742d35Cc6634C0532925a3b844Bc454e4438f44e"
	statusResponse, err := client.CheckTransactionStatus(ctx, sagapay.TransactionTypeDeposit, sagapay.CheckTransactionStatusOptions{Address: address})
	if err != nil {
		log.Fatalf("Failed to check transaction status: %v", err)
	}

	fmt.Println("✓ Transaction status retrieved:")
	fmt.Printf("  Address: %s\n", statusResponse.Address)
	fmt.Printf("  Transaction Type: %s\n", statusResponse.TransactionType)
	fmt.Printf("  Count: %d\n", statusResponse.Count)

	if statusResponse.Count > 0 {
		fmt.Println("  Transactions:")
		for i, tx := range statusResponse.Transactions {
			fmt.Printf("    #%d ID: %s, Status: %s, Amount: %s\n", i+1, tx.ID, tx.Status, tx.Amount)
		}
	}
	fmt.Println()

	// Example 4: Fetch wallet balance
	fmt.Println("Fetching wallet balance...")
	balanceResponse, err := client.FetchWalletBalance(
		ctx,
		"0x742d35Cc6634C0532925a3b844Bc454e4438f44e",
		sagapay.NetworkTypeERC20,
		"0xdAC17F958D2ee523a2206206994597C13D831ec7", // USDT on Ethereum
	)
	if err != nil {
		log.Fatalf("Failed to fetch wallet balance: %v", err)
	}

	fmt.Println("✓ Wallet balance retrieved:")
	fmt.Printf("  Address: %s\n", balanceResponse.Address)
	fmt.Printf("  Token: %s (%s)\n", balanceResponse.Token.Symbol, balanceResponse.Token.Name)
	fmt.Printf("  Balance: %s\n", balanceResponse.Balance.Formatted)
}
