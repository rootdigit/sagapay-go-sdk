// Package sagapay provides a Go client for the SagaPay blockchain payment gateway API.
//
// SagaPay is the world's first free, non-custodial blockchain payment gateway service provider.
// This package enables Go developers to integrate cryptocurrency payments without holding customer funds.
package sagapay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	// DefaultBaseURL is the default base URL for the SagaPay API
	DefaultBaseURL = "https://api2.sagapay.net"

	// DefaultTimeout is the default timeout for API requests
	DefaultTimeout = 30 * time.Second
)

// Client is the SagaPay API client
type Client struct {
	// HTTP client used to communicate with the API
	client *http.Client

	// Base URL for API requests
	baseURL *url.URL

	// API credentials
	apiKey    string
	apiSecret string
}

// Config contains the configuration options for the SagaPay client
type Config struct {
	// BaseURL is the base URL for the SagaPay API
	BaseURL string

	// APIKey is your SagaPay API key
	APIKey string

	// APISecret is your SagaPay API secret
	APISecret string

	// Timeout is the timeout for API requests
	Timeout time.Duration

	// HTTPClient is the HTTP client to use for API requests
	HTTPClient *http.Client
}

// NewClient creates a new SagaPay API client
func NewClient(config Config) (*Client, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	if config.APISecret == "" {
		return nil, fmt.Errorf("API secret is required")
	}

	baseURL := DefaultBaseURL
	if config.BaseURL != "" {
		baseURL = config.BaseURL
	}

	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		timeout := DefaultTimeout
		if config.Timeout > 0 {
			timeout = config.Timeout
		}
		httpClient = &http.Client{
			Timeout: timeout,
		}
	}

	return &Client{
		client:    httpClient,
		baseURL:   parsedURL,
		apiKey:    config.APIKey,
		apiSecret: config.APISecret,
	}, nil
}

// CreateDeposit creates a new deposit address for receiving cryptocurrency
func (c *Client) CreateDeposit(ctx context.Context, params CreateDepositParams) (*DepositResponse, error) {
	endpoint := "/create-deposit"

	// Validate params
	if err := params.Validate(); err != nil {
		return nil, err
	}

	var response DepositResponse
	err := c.sendRequest(ctx, http.MethodPost, endpoint, params, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

// CreateWithdrawal creates a cryptocurrency withdrawal request
func (c *Client) CreateWithdrawal(ctx context.Context, params CreateWithdrawalParams) (*WithdrawalResponse, error) {
	endpoint := "/create-withdrawal"

	// Validate params
	if err := params.Validate(); err != nil {
		return nil, err
	}

	var response WithdrawalResponse
	err := c.sendRequest(ctx, http.MethodPost, endpoint, params, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

// CheckTransactionStatusOptions holds optional parameters for CheckTransactionStatus
type CheckTransactionStatusOptions struct {
	Address string
	ID      string
}

// CheckTransactionStatus gets the status of transactions by address or ID
func (c *Client) CheckTransactionStatus(ctx context.Context, transactionType TransactionType, opts CheckTransactionStatusOptions) (*TransactionStatusResponse, error) {
	endpoint := "/check-transaction-status"

	if opts.Address == "" && opts.ID == "" {
		return nil, fmt.Errorf("either address or id is required")
	}

	// Build query parameters
	queryParams := url.Values{}
	queryParams.Add("type", string(transactionType))
	if opts.Address != "" {
		queryParams.Add("address", opts.Address)
	}
	if opts.ID != "" {
		queryParams.Add("id", opts.ID)
	}

	var response TransactionStatusResponse
	err := c.sendRequestWithQuery(ctx, http.MethodGet, endpoint, queryParams, nil, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

// FetchWalletBalance gets the balance of a specific wallet address for a token or native currency
func (c *Client) FetchWalletBalance(ctx context.Context, address string, networkType NetworkType, contractAddress string) (*WalletBalanceResponse, error) {
	endpoint := "/fetch-wallet-balance"

	// Validate params
	if address == "" {
		return nil, fmt.Errorf("address is required")
	}
	if networkType == "" {
		return nil, fmt.Errorf("networkType is required")
	}

	// Build query parameters
	queryParams := url.Values{}
	queryParams.Add("address", address)
	queryParams.Add("networkType", string(networkType))
	if contractAddress != "" {
		queryParams.Add("contractAddress", contractAddress)
	}

	var response WalletBalanceResponse
	err := c.sendRequestWithQuery(ctx, http.MethodGet, endpoint, queryParams, nil, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

// VerifyIPN verifies an IPN notification against the SagaPay API. This is the
// primary way to confirm that a received webhook notification is genuine.
//
// The verify-ipn endpoint takes the API credentials in the request body rather
// than in headers, so this request is sent without the authentication headers.
func (c *Client) VerifyIPN(ctx context.Context, params VerifyIPNParams) (*VerifyIPNResponse, error) {
	endpoint := "/verify-ipn"

	// Validate params
	if err := params.Validate(); err != nil {
		return nil, err
	}

	// Inject the client credentials into the request body
	body := struct {
		VerifyIPNParams
		APIKey    string `json:"apiKey"`
		APISecret string `json:"apiSecret"`
	}{
		VerifyIPNParams: params,
		APIKey:          c.apiKey,
		APISecret:       c.apiSecret,
	}

	var response VerifyIPNResponse
	err := c.doRequest(ctx, http.MethodPost, endpoint, nil, body, &response, false)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

// sendRequest sends an authenticated API request and parses the response
func (c *Client) sendRequest(ctx context.Context, method, path string, body interface{}, v interface{}) error {
	return c.sendRequestWithQuery(ctx, method, path, nil, body, v)
}

// sendRequestWithQuery sends an authenticated API request with query parameters and parses the response
func (c *Client) sendRequestWithQuery(ctx context.Context, method, path string, query url.Values, body interface{}, v interface{}) error {
	return c.doRequest(ctx, method, path, query, body, v, true)
}

// doRequest sends an API request and parses the response. When withAuth is
// false, the x-api-key/x-api-secret headers are omitted (used by endpoints
// such as verify-ipn that take the credentials in the request body).
func (c *Client) doRequest(ctx context.Context, method, path string, query url.Values, body interface{}, v interface{}, withAuth bool) error {
	// Create the request URL
	u, err := url.Parse(path)
	if err != nil {
		return err
	}

	u = c.baseURL.ResolveReference(u)

	// Add query parameters if any
	if query != nil {
		u.RawQuery = query.Encode()
	}

	// Create the request body if any
	var buf io.ReadWriter
	if body != nil {
		buf = new(bytes.Buffer)
		if err := json.NewEncoder(buf).Encode(body); err != nil {
			return err
		}
	}

	// Create the HTTP request
	req, err := http.NewRequestWithContext(ctx, method, u.String(), buf)
	if err != nil {
		return err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if withAuth {
		req.Header.Set("x-api-key", c.apiKey)
		req.Header.Set("x-api-secret", c.apiSecret)
	}

	// Send the request
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Parse the response
	if resp.StatusCode >= 400 {
		var apiErr APIError
		if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
			return fmt.Errorf("HTTP error: %d - failed to parse error response", resp.StatusCode)
		}
		apiErr.Code = resp.StatusCode
		return &apiErr
	}

	if v != nil {
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			return err
		}
	}

	return nil
}
