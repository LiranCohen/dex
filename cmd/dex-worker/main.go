// dex-worker is a standalone worker binary that executes objectives for Dex HQ.
// It can run in two modes:
//   - subprocess: Spawned by HQ, communicates via stdin/stdout
//   - standalone: Connects to HQ via mesh network
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/lirancohen/dex/internal/crypto"
	"github.com/lirancohen/dex/internal/worker"
)

const version = "0.1.0-dev"

func main() {
	// Define flags
	mode := flag.String("mode", "subprocess", "Worker mode: subprocess (stdin/stdout) or mesh (network)")
	id := flag.String("id", "", "Worker ID (auto-generated if not provided)")
	dataDir := flag.String("data-dir", "", "Worker data directory (for local database, identity)")
	hqPublicKey := flag.String("hq-public-key", "", "HQ's public key for encrypting responses")
	meshControlURL := flag.String("mesh-control-url", "https://central.enbox.id", "Mesh control server URL (mesh mode only)")
	meshAuthKey := flag.String("mesh-auth-key", "", "Mesh auth key (mesh mode only)")
	hqAddress := flag.String("hq-address", "", "HQ mesh address to connect to (mesh mode only)")
	showVersion := flag.Bool("version", false, "Show version and exit")

	flag.Parse()

	if *showVersion {
		fmt.Printf("dex-worker v%s\n", version)
		os.Exit(0)
	}

	// Determine data directory
	if *dataDir == "" {
		home, _ := os.UserHomeDir()
		*dataDir = filepath.Join(home, ".dex-worker")
	}

	// Ensure data directory exists
	if err := os.MkdirAll(*dataDir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create data directory: %v\n", err)
		os.Exit(1)
	}

	// Load or create worker identity
	workerID := *id
	if workerID == "" {
		hostname, _ := os.Hostname()
		workerID = fmt.Sprintf("worker-%s", hostname)
	}

	identityPath := filepath.Join(*dataDir, "identity.json")
	identity, err := crypto.EnsureWorkerIdentity(identityPath, workerID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load/create identity: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Worker %s starting (mode: %s)\n", identity.ID, *mode)
	fmt.Fprintf(os.Stderr, "Public key: %s\n", identity.PublicKey())

	// Set up context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Fprintf(os.Stderr, "\nReceived shutdown signal\n")
		cancel()
	}()

	// Run in appropriate mode
	switch *mode {
	case "subprocess":
		runSubprocessMode(ctx, identity, *dataDir, *hqPublicKey)
	case "mesh":
		runMeshMode(ctx, identity, *dataDir, *meshControlURL, *meshAuthKey, *hqAddress)
	default:
		fmt.Fprintf(os.Stderr, "Unknown mode: %s\n", *mode)
		os.Exit(1)
	}
}

// runSubprocessMode runs the worker in subprocess mode, communicating via stdin/stdout.
func runSubprocessMode(ctx context.Context, identity *crypto.WorkerIdentity, dataDir, hqPublicKey string) {
	// Create protocol connection over stdin/stdout
	conn := worker.NewConn(os.Stdin, os.Stdout)

	// Create receiver for decrypting payloads
	receiver := worker.NewReceiver(identity)

	// Open local database
	dbPath := filepath.Join(dataDir, "worker.db")
	localDB, err := worker.OpenLocalDB(dbPath, nil) // TODO: Add encryption
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open local database: %v\n", err)
		os.Exit(1)
	}
	defer localDB.Close()

	// Create worker runner
	runner := &workerRunner{
		conn:        conn,
		receiver:    receiver,
		identity:    identity,
		localDB:     localDB,
		hqPublicKey: hqPublicKey,
	}

	// Send ready message
	if err := conn.SendReady(identity.ID, version, identity.PublicKey()); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to send ready: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Worker ready, waiting for objectives...\n")

	// Run the main loop
	if err := runner.run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Worker error: %v\n", err)
		os.Exit(1)
	}
}

// runMeshMode runs the worker in mesh mode, connecting to HQ over the network.
func runMeshMode(ctx context.Context, identity *crypto.WorkerIdentity, dataDir, controlURL, authKey, hqAddress string) {
	// TODO: Implement mesh mode
	// 1. Connect to mesh network
	// 2. Dial HQ
	// 3. Send enrollment/ready message
	// 4. Enter message loop

	fmt.Fprintf(os.Stderr, "Mesh mode not yet implemented\n")
	os.Exit(1)
}

// workerRunner handles the main worker loop.
type workerRunner struct {
	conn        *worker.Conn
	receiver    *worker.Receiver
	identity    *crypto.WorkerIdentity
	localDB     *worker.LocalDB
	hqPublicKey string

	currentObjective *worker.ObjectivePayload
	currentSessionID string
}

// run executes the main worker loop.
func (r *workerRunner) run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		msg, err := r.conn.Receive()
		if err != nil {
			return fmt.Errorf("receive error: %w", err)
		}

		if err := r.handleMessage(ctx, msg); err != nil {
			// Send error to HQ but continue running
			_ = r.conn.SendError("handler_error", err.Error())
		}
	}
}

// handleMessage processes a message from HQ.
func (r *workerRunner) handleMessage(ctx context.Context, msg *worker.Message) error {
	switch msg.Type {
	case worker.MsgTypeDispatch:
		return r.handleDispatch(ctx, msg)
	case worker.MsgTypeCancel:
		return r.handleCancel(ctx, msg)
	case worker.MsgTypePing:
		return r.handlePing(ctx)
	case worker.MsgTypeShutdown:
		return r.handleShutdown(ctx)
	default:
		return fmt.Errorf("unknown message type: %s", msg.Type)
	}
}

// handleDispatch handles a dispatch message.
func (r *workerRunner) handleDispatch(ctx context.Context, msg *worker.Message) error {
	payload, err := worker.ParsePayload[worker.DispatchPayload](msg)
	if err != nil {
		return fmt.Errorf("failed to parse dispatch payload: %w", err)
	}

	objective := payload.Objective
	r.currentObjective = objective

	// Decrypt secrets
	secrets, err := r.receiver.DecryptPayload(objective)
	if err != nil {
		return fmt.Errorf("failed to decrypt secrets: %w", err)
	}

	// Store objective in local DB
	if err := r.localDB.StoreObjective(objective); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to store objective locally: %v\n", err)
	}

	// Generate session ID
	r.currentSessionID = fmt.Sprintf("sess-%d", ctx.Value("start_time"))
	if r.currentSessionID == "sess-<nil>" {
		r.currentSessionID = fmt.Sprintf("sess-%s", objective.Objective.ID[:8])
	}

	// Send accepted message
	if err := r.conn.SendAccepted(objective.Objective.ID, r.currentSessionID); err != nil {
		return fmt.Errorf("failed to send accepted: %w", err)
	}

	// Execute the objective
	// TODO: This is where we'd call the Ralph loop
	// For now, just simulate execution
	fmt.Fprintf(os.Stderr, "Executing objective: %s\n", objective.Objective.Title)
	fmt.Fprintf(os.Stderr, "  Hat: %s\n", objective.Objective.Hat)
	fmt.Fprintf(os.Stderr, "  Secrets decrypted: anthropic_key=%v, github_token=%v\n",
		secrets.AnthropicKey != "", secrets.GitHubToken != "")

	// TODO: Actually run the Ralph loop here
	// For now, send a completion (this is a stub)
	report := &worker.CompletionReport{
		ObjectiveID: objective.Objective.ID,
		SessionID:   r.currentSessionID,
		Status:      "completed",
		Summary:     "Objective execution completed (stub)",
		CompletedAt: msg.Timestamp,
	}

	if err := r.conn.SendCompleted(report); err != nil {
		return fmt.Errorf("failed to send completed: %w", err)
	}

	r.currentObjective = nil
	r.currentSessionID = ""

	return nil
}

// handleCancel handles a cancel message.
func (r *workerRunner) handleCancel(ctx context.Context, msg *worker.Message) error {
	payload, err := worker.ParsePayload[worker.CancelPayload](msg)
	if err != nil {
		return fmt.Errorf("failed to parse cancel payload: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Cancelling objective: %s (reason: %s)\n", payload.ObjectiveID, payload.Reason)

	// TODO: Actually cancel the running session

	return r.conn.Send(worker.MsgTypeCancelled, nil)
}

// handlePing handles a ping message.
func (r *workerRunner) handlePing(ctx context.Context) error {
	state := worker.WorkerStateIdle
	objectiveID := ""
	if r.currentObjective != nil {
		state = worker.WorkerStateRunning
		objectiveID = r.currentObjective.Objective.ID
	}

	return r.conn.SendPong(&worker.PongPayload{
		WorkerID:    r.identity.ID,
		State:       state,
		ObjectiveID: objectiveID,
	})
}

// handleShutdown handles a shutdown message.
func (r *workerRunner) handleShutdown(ctx context.Context) error {
	fmt.Fprintf(os.Stderr, "Shutdown requested\n")

	// Send acknowledgment
	if err := r.conn.Send(worker.MsgTypeShutdownAck, nil); err != nil {
		return err
	}

	// Exit cleanly
	os.Exit(0)
	return nil
}
