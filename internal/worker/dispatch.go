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
// workerPublicKey should be an age X25519 public key string (age1...).
func (d *Dispatcher) PreparePayload(
	objective Objective,
	project Project,
	secrets WorkerSecrets,
	workerPublicKey string,
	syncConfig SyncConfig,
) (*ObjectivePayload, error) {
	// Serialize secrets to JSON
	secretsJSON, err := json.Marshal(secrets)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal secrets: %w", err)
	}

	// Encrypt secrets for the worker using age
	encryptedSecrets, err := d.hqKeyPair.EncryptForRecipient(secretsJSON, workerPublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt secrets: %w", err)
	}

	return &ObjectivePayload{
		Objective:        objective,
		Project:          project,
		SecretsEncrypted: encryptedSecrets,
		Sync:             syncConfig,
		DispatchedAt:     time.Now(),
		HQPublicKey:      d.hqKeyPair.PublicKey(),
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

	// Decrypt using the worker's identity
	decrypted, err := r.workerIdentity.Decrypt(payload.SecretsEncrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt secrets: %w", err)
	}

	if err := json.Unmarshal(decrypted, &secrets); err != nil {
		return nil, fmt.Errorf("failed to parse secrets: %w", err)
	}

	return &secrets, nil
}

// EncryptResponse encrypts data to send back to HQ.
// Uses HQ's public key from the payload.
func (r *Receiver) EncryptResponse(data []byte, hqPublicKey string) (string, error) {
	if hqPublicKey == "" {
		return "", fmt.Errorf("HQ public key not provided")
	}

	kp := r.workerIdentity.ToKeyPair()
	if kp == nil {
		return "", fmt.Errorf("worker identity has no private key")
	}
	return kp.EncryptForRecipient(data, hqPublicKey)
}

// EncryptCompletionReport encrypts a completion report for HQ.
func (r *Receiver) EncryptCompletionReport(report *CompletionReport, hqPublicKey string) (string, error) {
	data, err := json.Marshal(report)
	if err != nil {
		return "", fmt.Errorf("failed to marshal report: %w", err)
	}
	return r.EncryptResponse(data, hqPublicKey)
}

// DecryptSecrets decrypts an encrypted secrets string (used for resumption).
func (r *Receiver) DecryptSecrets(encryptedSecrets string) (*WorkerSecrets, error) {
	var secrets WorkerSecrets

	decrypted, err := r.workerIdentity.Decrypt(encryptedSecrets)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt secrets: %w", err)
	}

	if err := json.Unmarshal(decrypted, &secrets); err != nil {
		return nil, fmt.Errorf("failed to parse secrets: %w", err)
	}

	return &secrets, nil
}
