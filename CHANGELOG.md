# Changelog

## v1.1.0 — 2026-10-27

### Security
- Webhooks are verified with `X-Sagapay-Merchant-Signature`, keyed with your own signing secret (dashboard: Webhooks → Signing secret). `HandleRequest` no longer reads the retired `X-Sagapay-Signature` header, which was keyed with a secret shared by every merchant.
- A `WebhookHandler` created with an empty secret now rejects every notification. Previously it skipped signature verification.
- `VerifySignature` compares the decoded digest, accepts upper-case hex, and rejects a malformed digest. `HandleRequest` rejects a repeated signature header and bodies over 1 MiB.
- The default HTTP client no longer follows redirects. net/http resends custom headers such as `x-api-key` and `x-api-secret` on a redirect. If you pass your own `HTTPClient`, set its `CheckRedirect` to return `http.ErrUseLastResponse`.

### Added
- `IsFresh(payload, maxAge, now)` and `WebhookHandler.WithMaxAge(maxAge)` reject notifications whose `timestamp` is too old or more than 5 minutes in the future. Use them alongside processing each `id` only once.
- `SignatureHeader` and `DefaultMaxAge` constants.
- `CreateWithdrawalParams.IdempotencyKey` is sent as an `Idempotency-Key` header, so a retried request cannot create a second withdrawal.
- `WebhookPayload` has the `Asset` and `ContractAddress` fields that notifications carry.
