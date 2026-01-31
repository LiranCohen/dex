# Authentication Migration Notes

## What Changed

### Old Way (BIP39 Seed Phrase)
- Setup wizard generated a 24-word BIP39 mnemonic
- User had to save/backup the phrase securely
- Login required entering the phrase or derived signature
- Ed25519 keypair derived from mnemonic
- Challenge-response auth: server sends challenge, client signs with private key

### New Way (Passkeys/WebAuthn)
- Setup wizard only collects API keys (Anthropic, GitHub)
- First visit to app prompts passkey registration
- Uses device biometrics (Face ID, Touch ID) or security keys
- No phrases to remember or backup
- Credentials stored securely in device/browser

## Files Changed

### Removed/Modified - Passphrase Code
- `cmd/dex-setup/main.go` - Removed BIP39 generation, simplified Secrets struct
- `cmd/dex-setup/static/index.html` - Removed passphrase display screen
- `cmd/dex-setup/setup_test.go` - Updated tests for new structure
- `scripts/install.sh` - Updated messaging about passkey setup

### Added - Passkey Support
- `internal/auth/passkey.go` - WebAuthn configuration and user adapter
- `internal/api/passkey.go` - Registration and login endpoints
- `internal/db/credentials.go` - WebAuthnCredential storage
- `frontend/src/App.tsx` - Passkey registration/login UI

### API Endpoints

**New Passkey Endpoints:**
- `GET /api/v1/auth/passkey/status` - Check if passkeys configured
- `POST /api/v1/auth/passkey/register/begin` - Start registration
- `POST /api/v1/auth/passkey/register/finish` - Complete registration
- `POST /api/v1/auth/passkey/login/begin` - Start login
- `POST /api/v1/auth/passkey/login/finish` - Complete login

**Legacy Endpoints (still exist but less prominent):**
- `POST /api/v1/auth/challenge` - Get challenge for Ed25519 auth
- `POST /api/v1/auth/verify` - Verify Ed25519 signature

## User Flow

### Setup (One-time)
1. Run installer: `curl -fsSL https://server/install.sh | bash`
2. Scan QR to join Tailscale
3. Scan QR to open setup wizard
4. Enter Anthropic API key and GitHub token
5. Visit app URL, click "Set up Passkey"
6. Authenticate with Face ID/Touch ID/security key
7. Done - logged in with JWT

### Login (Subsequent)
1. Visit app URL
2. Click "Sign in with Passkey"
3. Authenticate with biometric
4. Done - logged in

## Dependencies

- `github.com/go-webauthn/webauthn` v0.15.0 - WebAuthn server library
- `github.com/tyler-smith/go-bip39` - **REMOVED** (no longer needed)

## Removed Files

- `internal/auth/bip39.go` - BIP39 mnemonic generation (deleted)
- `DeriveKeypair()` in `internal/auth/ed25519.go` - keypair derivation from mnemonic (deleted)

## Kept for Legacy Support

- `Sign()` and `Verify()` in `internal/auth/ed25519.go` - still used by challenge-response auth endpoint
- Challenge-response auth endpoints (`/auth/challenge`, `/auth/verify`) - still functional but not primary auth

## Notes

- JWT tokens are generated with ED25519 keys (generated fresh on each server start)
- Single-user mode: only one user can register (first passkey wins)
- Passkey credentials stored in SQLite `webauthn_credentials` table
- RPID is derived from request Host header for flexibility
