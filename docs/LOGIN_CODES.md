# Player registration codes

The private GM console generates single-use `DNF-XXXX-XXXX-XXXX-XXXX` registration codes. Only SHA-256 digests and the visible prefix are stored. The plaintext list is returned once from `POST /api/v1/login-codes` and is not written to application logs.

The player gateway requires a code for `POST /api/account/register`. It reserves the code for the submitted account before forwarding the existing registration request, strips the code before forwarding, consumes it only when the upstream returns HTTP success and JSON `ok: true`, and releases it after a definitive rejection. A 5xx, malformed acknowledgement, or lost upstream connection leaves the code reserved to that account so the same account can retry without exposing the code to another registration. After checking the account in GM, an operator can resolve a reservation older than five minutes as consumed; reserved codes cannot be unbound while registration might still be in flight.

Management routes:

- `POST /api/v1/login-codes` — generate 1–1000 single-use codes (authenticated GM session and CSRF required).
- `GET /api/v1/login-codes` — paginated, prefix-only list.
- `POST /api/v1/login-codes/{id}/revoke` — stop future use.
- `POST /api/v1/login-codes/{id}/unbind` — release an active or revoked code and return it to `issued`.
- `POST /api/v1/login-codes/{id}/resolve` — after verifying the account exists, consume a reserved code left by an ambiguous upstream result.
- `GET /api/v1/login-codes/{id}/audit` — per-code administrative and registration events.

After deploying code, run the existing explicit additive-schema step once with the production management configuration: `playergateway --config <private-config> --init-management-schema`. This creates the two `dnf90_web_accounts_login_codes*` tables in the registration database; it does not modify game tables. Then rebuild and restart the gateway/GM services. Do not run prepare against `data2`.

The Flutter registration form sends the code through its private host RPC. The gateway consumes it and removes it before passing account credentials to the existing game account API. Remembered account passwords are stored only as a Windows DPAPI-protected local blob; they are not included in preferences JSON or diagnostic records.
