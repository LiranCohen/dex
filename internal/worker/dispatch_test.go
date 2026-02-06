package worker

import (
	"testing"
	"time"

	"github.com/lirancohen/dex/internal/crypto"
)

func TestDispatcher_PreparePayload_E2E(t *testing.T) {
	// Generate HQ and Worker keypairs (simulating real deployment)
	hqKeyPair, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate HQ keypair: %v", err)
	}

	workerIdentity, err := crypto.NewWorkerIdentity("worker-001")
	if err != nil {
		t.Fatalf("Failed to create worker identity: %v", err)
	}

	// Create dispatcher (runs on HQ)
	dispatcher := NewDispatcher(hqKeyPair)

	// Create test objective, project, and secrets
	objective := Objective{
		ID:          "obj-test-123",
		Title:       "Implement feature X",
		Description: "Add the new feature with full test coverage",
		Hat:         "engineer",
		BaseBranch:  "main",
		TokenBudget: 50000,
		Checklist:   []string{"Write code", "Add tests", "Update docs"},
	}

	project := Project{
		ID:          "proj-456",
		Name:        "test-project",
		GitHubOwner: "testorg",
		GitHubRepo:  "test-repo",
		CloneURL:    "https://github.com/testorg/test-repo.git",
	}

	secrets := WorkerSecrets{
		AnthropicKey:    "sk-ant-api03-xxxxx",
		GitHubToken:     "ghp_xxxxxxxxxxxx",
		FlyToken:        "fo1_xxxxx",
		CloudflareToken: "cf_xxxxx",
	}

	syncConfig := SyncConfig{
		HQEndpoint:           "100.100.1.1:8080",
		ActivityIntervalSec:  30,
		HeartbeatIntervalSec: 10,
	}

	// HQ prepares encrypted payload for worker
	payload, err := dispatcher.PreparePayload(objective, project, secrets, workerIdentity.PublicKey(), syncConfig)
	if err != nil {
		t.Fatalf("PreparePayload failed: %v", err)
	}

	// Verify payload structure
	if payload.Objective.ID != objective.ID {
		t.Errorf("Objective ID mismatch: got %q, want %q", payload.Objective.ID, objective.ID)
	}
	if payload.Project.Name != project.Name {
		t.Errorf("Project name mismatch: got %q, want %q", payload.Project.Name, project.Name)
	}
	if payload.SecretsEncrypted == "" {
		t.Error("SecretsEncrypted should not be empty")
	}
	if payload.HQPublicKey == "" {
		t.Error("HQPublicKey should not be empty")
	}
	if payload.HQPublicKey != hqKeyPair.PublicKey() {
		t.Error("HQPublicKey should match dispatcher's public key")
	}
	if payload.DispatchedAt.IsZero() {
		t.Error("DispatchedAt should be set")
	}

	// Create receiver (runs on worker)
	receiver := NewReceiver(workerIdentity)

	// Worker decrypts the payload
	decryptedSecrets, err := receiver.DecryptPayload(payload)
	if err != nil {
		t.Fatalf("DecryptPayload failed: %v", err)
	}

	// Verify decrypted secrets match
	if decryptedSecrets.AnthropicKey != secrets.AnthropicKey {
		t.Errorf("AnthropicKey mismatch: got %q, want %q", decryptedSecrets.AnthropicKey, secrets.AnthropicKey)
	}
	if decryptedSecrets.GitHubToken != secrets.GitHubToken {
		t.Errorf("GitHubToken mismatch: got %q, want %q", decryptedSecrets.GitHubToken, secrets.GitHubToken)
	}
	if decryptedSecrets.FlyToken != secrets.FlyToken {
		t.Errorf("FlyToken mismatch: got %q, want %q", decryptedSecrets.FlyToken, secrets.FlyToken)
	}
	if decryptedSecrets.CloudflareToken != secrets.CloudflareToken {
		t.Errorf("CloudflareToken mismatch: got %q, want %q", decryptedSecrets.CloudflareToken, secrets.CloudflareToken)
	}
}

func TestDispatcher_WrongWorkerCannotDecrypt(t *testing.T) {
	hqKeyPair, _ := crypto.GenerateKeyPair()
	intendedWorker, _ := crypto.NewWorkerIdentity("worker-001")
	attackerWorker, _ := crypto.NewWorkerIdentity("attacker-001")

	dispatcher := NewDispatcher(hqKeyPair)

	secrets := WorkerSecrets{
		AnthropicKey: "sk-ant-api03-secret-key",
		GitHubToken:  "ghp_secret_token",
	}

	// Encrypt for intended worker
	payload, err := dispatcher.PreparePayload(
		Objective{ID: "obj-1", Title: "Test", Hat: "engineer"},
		Project{ID: "proj-1", Name: "test"},
		secrets,
		intendedWorker.PublicKey(),
		SyncConfig{},
	)
	if err != nil {
		t.Fatalf("PreparePayload failed: %v", err)
	}

	// Attacker tries to decrypt
	attackerReceiver := NewReceiver(attackerWorker)
	_, err = attackerReceiver.DecryptPayload(payload)
	if err == nil {
		t.Error("Attacker should NOT be able to decrypt payload intended for different worker")
	}

	// Intended worker should succeed
	intendedReceiver := NewReceiver(intendedWorker)
	decrypted, err := intendedReceiver.DecryptPayload(payload)
	if err != nil {
		t.Fatalf("Intended worker should be able to decrypt: %v", err)
	}
	if decrypted.AnthropicKey != secrets.AnthropicKey {
		t.Error("Decrypted secrets mismatch")
	}
}

func TestReceiver_EncryptResponse(t *testing.T) {
	// Setup HQ and Worker
	hqKeyPair, _ := crypto.GenerateKeyPair()
	workerIdentity, _ := crypto.NewWorkerIdentity("worker-001")

	receiver := NewReceiver(workerIdentity)

	// Worker encrypts response data for HQ
	responseData := []byte(`{"status": "completed", "pr_url": "https://github.com/org/repo/pull/42"}`)
	encrypted, err := receiver.EncryptResponse(responseData, hqKeyPair.PublicKey())
	if err != nil {
		t.Fatalf("EncryptResponse failed: %v", err)
	}

	if encrypted == "" {
		t.Error("Encrypted response should not be empty")
	}
	if encrypted == string(responseData) {
		t.Error("Encrypted should differ from plaintext")
	}

	// HQ decrypts the response
	decrypted, err := hqKeyPair.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("HQ failed to decrypt worker response: %v", err)
	}

	if string(decrypted) != string(responseData) {
		t.Errorf("Response data mismatch: got %q, want %q", decrypted, responseData)
	}
}

func TestReceiver_EncryptCompletionReport(t *testing.T) {
	hqKeyPair, _ := crypto.GenerateKeyPair()
	workerIdentity, _ := crypto.NewWorkerIdentity("worker-001")

	receiver := NewReceiver(workerIdentity)

	report := &CompletionReport{
		ObjectiveID: "obj-123",
		SessionID:   "sess-456",
		Status:      "completed",
		Summary:     "Successfully implemented feature X",
		PRNumber:    42,
		PRURL:       "https://github.com/org/repo/pull/42",
		BranchName:  "feature/x",
		TotalTokens: 25000,
		TotalCost:   0.50,
		Iterations:  15,
		CompletedAt: time.Now(),
	}

	encrypted, err := receiver.EncryptCompletionReport(report, hqKeyPair.PublicKey())
	if err != nil {
		t.Fatalf("EncryptCompletionReport failed: %v", err)
	}

	// HQ decrypts
	decrypted, err := hqKeyPair.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("HQ failed to decrypt completion report: %v", err)
	}

	// Verify it contains expected data (JSON)
	if len(decrypted) == 0 {
		t.Error("Decrypted report should not be empty")
	}
}

func TestReceiver_EncryptResponseNoHQKey(t *testing.T) {
	workerIdentity, _ := crypto.NewWorkerIdentity("worker-001")
	receiver := NewReceiver(workerIdentity)

	_, err := receiver.EncryptResponse([]byte("data"), "")
	if err == nil {
		t.Error("Should fail when HQ public key is empty")
	}
}

func TestDispatcher_InvalidWorkerKey(t *testing.T) {
	hqKeyPair, _ := crypto.GenerateKeyPair()
	dispatcher := NewDispatcher(hqKeyPair)

	_, err := dispatcher.PreparePayload(
		Objective{ID: "obj-1"},
		Project{ID: "proj-1"},
		WorkerSecrets{AnthropicKey: "key"},
		"invalid-not-age-key",
		SyncConfig{},
	)
	if err == nil {
		t.Error("Should fail with invalid worker public key")
	}
}

func TestFullCommunicationCycle(t *testing.T) {
	// This test simulates a complete HQ <-> Worker communication cycle
	// 1. HQ creates and encrypts payload for worker
	// 2. Worker decrypts payload and processes
	// 3. Worker encrypts response for HQ
	// 4. HQ decrypts response

	// Setup identities
	hqKeyPair, _ := crypto.GenerateKeyPair()
	workerIdentity, _ := crypto.NewWorkerIdentity("worker-full-test")

	// === STEP 1: HQ prepares dispatch ===
	dispatcher := NewDispatcher(hqKeyPair)

	objective := Objective{
		ID:          "obj-full-cycle",
		Title:       "Full cycle test",
		Description: "Test complete encryption roundtrip",
		Hat:         "engineer",
	}
	project := Project{
		ID:   "proj-cycle",
		Name: "cycle-test",
	}
	secrets := WorkerSecrets{
		AnthropicKey: "sk-ant-real-key-xxxxx",
		GitHubToken:  "ghp_real_token_xxxxx",
	}

	payload, err := dispatcher.PreparePayload(objective, project, secrets, workerIdentity.PublicKey(), SyncConfig{
		HQEndpoint:           "100.100.1.1:8080",
		HeartbeatIntervalSec: 10,
	})
	if err != nil {
		t.Fatalf("Step 1: PreparePayload failed: %v", err)
	}

	// === STEP 2: Worker receives and decrypts ===
	receiver := NewReceiver(workerIdentity)

	decryptedSecrets, err := receiver.DecryptPayload(payload)
	if err != nil {
		t.Fatalf("Step 2: DecryptPayload failed: %v", err)
	}

	if decryptedSecrets.AnthropicKey != secrets.AnthropicKey {
		t.Fatalf("Step 2: Secrets mismatch after decrypt")
	}

	// Worker can now use decrypted credentials
	t.Logf("Worker received objective: %s", payload.Objective.Title)
	t.Logf("Worker has Anthropic key: %s...", decryptedSecrets.AnthropicKey[:10])

	// === STEP 3: Worker completes and sends encrypted report ===
	report := &CompletionReport{
		ObjectiveID: payload.Objective.ID,
		SessionID:   "sess-generated-by-worker",
		Status:      "completed",
		Summary:     "Successfully completed the objective",
		PRNumber:    99,
		PRURL:       "https://github.com/org/repo/pull/99",
		TotalTokens: 30000,
		TotalCost:   0.75,
		Iterations:  20,
		CompletedAt: time.Now(),
	}

	encryptedReport, err := receiver.EncryptCompletionReport(report, payload.HQPublicKey)
	if err != nil {
		t.Fatalf("Step 3: EncryptCompletionReport failed: %v", err)
	}

	// === STEP 4: HQ receives and decrypts response ===
	decryptedReportBytes, err := hqKeyPair.Decrypt(encryptedReport)
	if err != nil {
		t.Fatalf("Step 4: HQ failed to decrypt report: %v", err)
	}

	if len(decryptedReportBytes) == 0 {
		t.Fatal("Step 4: Decrypted report should not be empty")
	}

	t.Logf("Full cycle completed successfully")
}

func TestMultipleWorkersIsolation(t *testing.T) {
	// Test that secrets for one worker cannot be read by another
	hqKeyPair, _ := crypto.GenerateKeyPair()
	dispatcher := NewDispatcher(hqKeyPair)

	worker1, _ := crypto.NewWorkerIdentity("worker-1")
	worker2, _ := crypto.NewWorkerIdentity("worker-2")
	worker3, _ := crypto.NewWorkerIdentity("worker-3")

	secrets1 := WorkerSecrets{AnthropicKey: "key-for-worker-1"}
	secrets2 := WorkerSecrets{AnthropicKey: "key-for-worker-2"}
	secrets3 := WorkerSecrets{AnthropicKey: "key-for-worker-3"}

	payload1, _ := dispatcher.PreparePayload(Objective{ID: "obj-1"}, Project{}, secrets1, worker1.PublicKey(), SyncConfig{})
	payload2, _ := dispatcher.PreparePayload(Objective{ID: "obj-2"}, Project{}, secrets2, worker2.PublicKey(), SyncConfig{})
	payload3, _ := dispatcher.PreparePayload(Objective{ID: "obj-3"}, Project{}, secrets3, worker3.PublicKey(), SyncConfig{})

	receiver1 := NewReceiver(worker1)
	receiver2 := NewReceiver(worker2)
	receiver3 := NewReceiver(worker3)

	// Each worker can only decrypt their own payload
	dec1, err := receiver1.DecryptPayload(payload1)
	if err != nil || dec1.AnthropicKey != secrets1.AnthropicKey {
		t.Error("Worker 1 should decrypt payload 1")
	}

	dec2, err := receiver2.DecryptPayload(payload2)
	if err != nil || dec2.AnthropicKey != secrets2.AnthropicKey {
		t.Error("Worker 2 should decrypt payload 2")
	}

	dec3, err := receiver3.DecryptPayload(payload3)
	if err != nil || dec3.AnthropicKey != secrets3.AnthropicKey {
		t.Error("Worker 3 should decrypt payload 3")
	}

	// Cross-decryption should fail
	if _, err := receiver1.DecryptPayload(payload2); err == nil {
		t.Error("Worker 1 should NOT decrypt payload 2")
	}
	if _, err := receiver2.DecryptPayload(payload3); err == nil {
		t.Error("Worker 2 should NOT decrypt payload 3")
	}
	if _, err := receiver3.DecryptPayload(payload1); err == nil {
		t.Error("Worker 3 should NOT decrypt payload 1")
	}
}

func TestEmptySecrets(t *testing.T) {
	hqKeyPair, _ := crypto.GenerateKeyPair()
	workerIdentity, _ := crypto.NewWorkerIdentity("worker-empty")

	dispatcher := NewDispatcher(hqKeyPair)

	// Empty secrets should still work
	payload, err := dispatcher.PreparePayload(
		Objective{ID: "obj-1"},
		Project{ID: "proj-1"},
		WorkerSecrets{}, // empty
		workerIdentity.PublicKey(),
		SyncConfig{},
	)
	if err != nil {
		t.Fatalf("PreparePayload with empty secrets failed: %v", err)
	}

	receiver := NewReceiver(workerIdentity)
	decrypted, err := receiver.DecryptPayload(payload)
	if err != nil {
		t.Fatalf("DecryptPayload with empty secrets failed: %v", err)
	}

	if decrypted.AnthropicKey != "" {
		t.Error("AnthropicKey should be empty")
	}
}

func TestLargeSecrets(t *testing.T) {
	hqKeyPair, _ := crypto.GenerateKeyPair()
	workerIdentity, _ := crypto.NewWorkerIdentity("worker-large")

	dispatcher := NewDispatcher(hqKeyPair)

	// Create large secrets (e.g., long SSH key)
	largeToken := make([]byte, 10000)
	for i := range largeToken {
		largeToken[i] = byte('A' + (i % 26))
	}

	secrets := WorkerSecrets{
		AnthropicKey: string(largeToken),
		GitHubToken:  "normal-token",
	}

	payload, err := dispatcher.PreparePayload(
		Objective{ID: "obj-1"},
		Project{ID: "proj-1"},
		secrets,
		workerIdentity.PublicKey(),
		SyncConfig{},
	)
	if err != nil {
		t.Fatalf("PreparePayload with large secrets failed: %v", err)
	}

	receiver := NewReceiver(workerIdentity)
	decrypted, err := receiver.DecryptPayload(payload)
	if err != nil {
		t.Fatalf("DecryptPayload with large secrets failed: %v", err)
	}

	if decrypted.AnthropicKey != string(largeToken) {
		t.Error("Large secret should be preserved exactly")
	}
}

func TestReceiver_DecryptSecrets(t *testing.T) {
	// This tests the DecryptSecrets method used for session resumption
	hqKeyPair, _ := crypto.GenerateKeyPair()
	workerIdentity, _ := crypto.NewWorkerIdentity("worker-resume")

	dispatcher := NewDispatcher(hqKeyPair)
	receiver := NewReceiver(workerIdentity)

	// Create secrets and encrypt them as HQ would for resumption
	secrets := WorkerSecrets{
		AnthropicKey: "sk-ant-resume-key",
		GitHubToken:  "ghp_resume_token",
	}

	// Create a payload to get encrypted secrets
	payload, err := dispatcher.PreparePayload(
		Objective{ID: "obj-resume"},
		Project{},
		secrets,
		workerIdentity.PublicKey(),
		SyncConfig{},
	)
	if err != nil {
		t.Fatalf("PreparePayload failed: %v", err)
	}

	// Use DecryptSecrets (the method used during resumption)
	decrypted, err := receiver.DecryptSecrets(payload.SecretsEncrypted)
	if err != nil {
		t.Fatalf("DecryptSecrets failed: %v", err)
	}

	if decrypted.AnthropicKey != secrets.AnthropicKey {
		t.Errorf("AnthropicKey mismatch: got %q, want %q", decrypted.AnthropicKey, secrets.AnthropicKey)
	}
	if decrypted.GitHubToken != secrets.GitHubToken {
		t.Errorf("GitHubToken mismatch: got %q, want %q", decrypted.GitHubToken, secrets.GitHubToken)
	}
}

func TestReceiver_DecryptSecrets_InvalidData(t *testing.T) {
	workerIdentity, _ := crypto.NewWorkerIdentity("worker-invalid")
	receiver := NewReceiver(workerIdentity)

	// Try to decrypt invalid encrypted string
	_, err := receiver.DecryptSecrets("not-valid-encrypted-data")
	if err == nil {
		t.Error("DecryptSecrets should fail with invalid data")
	}
}

func TestReceiver_DecryptSecrets_WrongWorker(t *testing.T) {
	hqKeyPair, _ := crypto.GenerateKeyPair()
	worker1, _ := crypto.NewWorkerIdentity("worker-1")
	worker2, _ := crypto.NewWorkerIdentity("worker-2")

	dispatcher := NewDispatcher(hqKeyPair)

	secrets := WorkerSecrets{
		AnthropicKey: "secret-key",
	}

	// Encrypt for worker1
	payload, _ := dispatcher.PreparePayload(
		Objective{ID: "obj-1"},
		Project{},
		secrets,
		worker1.PublicKey(),
		SyncConfig{},
	)

	// Worker2 tries to decrypt
	receiver2 := NewReceiver(worker2)
	_, err := receiver2.DecryptSecrets(payload.SecretsEncrypted)
	if err == nil {
		t.Error("DecryptSecrets should fail when wrong worker tries to decrypt")
	}
}
