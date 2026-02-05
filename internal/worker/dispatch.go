package worker

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/lirancohen/dex/internal/crypto"
)

// Dispatcher handles encrypting and dispatching objectives to workers.
// It runs on HQ and uses worker public keys to encrypt payloads.
type Dispatcher struct {
	hqKeyPair *crypto.KeyPair
}

// NewDispatcher creates a new dispatcher with HQ's keypair.
func NewDispatcher(hqKeyPair *crypto.KeyPair) *Dispatcher {
	return &Dispatcher{
		hqKeyPair: hqKeyPair,
	}
}

// PreparePayload creates an encrypted ObjectivePayload for a specific worker.
// The secrets are encrypted with the worker's public key so only they can decrypt.
func (d *Dispatcher) PreparePayload(
	objective Objective,
	project Project,
	secrets WorkerSecrets,
	workerPublicKeyB64 string,
	syncConfig SyncConfig,
) (*ObjectivePayload, error) {
	// Parse worker's public key
	workerPubKey, err := crypto.PublicKeyFromBase64(workerPublicKeyB64)
	if err != nil {
		return nil, fmt.Errorf("invalid worker public key: %w", err)
	}

	// Serialize secrets to JSON
	secretsJSON, err := json.Marshal(secrets)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal secrets: %w", err)
	}

	// Encrypt secrets for the worker
	encryptedSecrets, err := d.hqKeyPair.EncryptForRecipient(secretsJSON, workerPubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt secrets: %w", err)
	}

	return &ObjectivePayload{
		Objective:        objective,
		Project:          project,
		SecretsEncrypted: encryptedSecrets,
		Sync:             syncConfig,
		DispatchedAt:     time.Now(),
		HQPublicKey:      d.hqKeyPair.PublicKeyBase64(),
	}, nil
}

// PreparePayloadAnonymous creates a payload encrypted without requiring HQ's private key.
// This uses anonymous encryption where the sender identity is not verified.
// Useful for one-way dispatch where workers don't need to verify HQ.
func PreparePayloadAnonymous(
	objective Objective,
	project Project,
	secrets WorkerSecrets,
	workerPublicKeyB64 string,
	syncConfig SyncConfig,
) (*ObjectivePayload, error) {
	// Parse worker's public key
	workerPubKey, err := crypto.PublicKeyFromBase64(workerPublicKeyB64)
	if err != nil {
		return nil, fmt.Errorf("invalid worker public key: %w", err)
	}

	// Serialize secrets to JSON
	secretsJSON, err := json.Marshal(secrets)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal secrets: %w", err)
	}

	// Encrypt secrets anonymously for the worker
	encryptedSecrets, err := crypto.SealAnonymous(secretsJSON, workerPubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt secrets: %w", err)
	}

	return &ObjectivePayload{
		Objective:        objective,
		Project:          project,
		SecretsEncrypted: encryptedSecrets,
		Sync:             syncConfig,
		DispatchedAt:     time.Now(),
		HQPublicKey:      "", // Not provided for anonymous encryption
	}, nil
}

// Receiver handles decrypting payloads received from HQ.
// It runs on workers and uses the worker's private key to decrypt.
type Receiver struct {
	workerIdentity *crypto.WorkerIdentity
}

// NewReceiver creates a new receiver with the worker's identity.
func NewReceiver(identity *crypto.WorkerIdentity) *Receiver {
	return &Receiver{
		workerIdentity: identity,
	}
}

// DecryptPayload decrypts the secrets from an ObjectivePayload.
func (r *Receiver) DecryptPayload(payload *ObjectivePayload) (*WorkerSecrets, error) {
	var secrets WorkerSecrets

	kp := r.workerIdentity.ToKeyPair()

	// Try authenticated decryption first if HQ public key is provided
	if payload.HQPublicKey != "" {
		hqPubKey, err := crypto.PublicKeyFromBase64(payload.HQPublicKey)
		if err != nil {
			return nil, fmt.Errorf("invalid HQ public key: %w", err)
		}

		decrypted, err := kp.DecryptFromSender(payload.SecretsEncrypted, hqPubKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt secrets: %w", err)
		}

		if err := json.Unmarshal(decrypted, &secrets); err != nil {
			return nil, fmt.Errorf("failed to parse secrets: %w", err)
		}

		return &secrets, nil
	}

	// Fall back to anonymous decryption
	decrypted, err := kp.OpenAnonymous(payload.SecretsEncrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt anonymous secrets: %w", err)
	}

	if err := json.Unmarshal(decrypted, &secrets); err != nil {
		return nil, fmt.Errorf("failed to parse secrets: %w", err)
	}

	return &secrets, nil
}

// EncryptResponse encrypts data to send back to HQ.
// Uses HQ's public key from the payload.
func (r *Receiver) EncryptResponse(data []byte, hqPublicKeyB64 string) (string, error) {
	if hqPublicKeyB64 == "" {
		return "", fmt.Errorf("HQ public key not provided")
	}

	hqPubKey, err := crypto.PublicKeyFromBase64(hqPublicKeyB64)
	if err != nil {
		return "", fmt.Errorf("invalid HQ public key: %w", err)
	}

	kp := r.workerIdentity.ToKeyPair()
	return kp.EncryptForRecipient(data, hqPubKey)
}

// EncryptCompletionReport encrypts a completion report for HQ.
func (r *Receiver) EncryptCompletionReport(report *CompletionReport, hqPublicKeyB64 string) (string, error) {
	data, err := json.Marshal(report)
	if err != nil {
		return "", fmt.Errorf("failed to marshal report: %w", err)
	}
	return r.EncryptResponse(data, hqPublicKeyB64)
}
