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
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/lirancohen/dex/internal/crypto"
	"github.com/lirancohen/dex/internal/toolbelt"
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

	// Load or create master key for local encryption
	masterKeyPath := filepath.Join(dataDir, "master.key")
	masterKey, err := crypto.EnsureMasterKey(masterKeyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize master key: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Encryption key loaded from %s\n", masterKeyPath)

	// Open local database with encryption
	dbPath := filepath.Join(dataDir, "worker.db")
	localDB, err := worker.OpenLocalDB(dbPath, masterKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open local database: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = localDB.Close() }()

	// Load prompts
	promptLoader := worker.NewWorkerPromptLoader()
	if err := promptLoader.LoadAll(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load prompts: %v\n", err)
		os.Exit(1)
	}

	// Create project manager
	projectManager := worker.NewProjectManager(dataDir)

	// Create worker runner
	runner := &workerRunner{
		conn:           conn,
		receiver:       receiver,
		identity:       identity,
		localDB:        localDB,
		hqPublicKey:    hqPublicKey,
		dataDir:        dataDir,
		promptLoader:   promptLoader,
		projectManager: projectManager,
		startedAt:      time.Now(),
	}

	// Check for incomplete sessions from previous run
	var crashedSession *worker.SessionState
	if incompleteSession, err := localDB.GetIncompleteSession(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to check for incomplete sessions: %v\n", err)
	} else if incompleteSession != nil {
		fmt.Fprintf(os.Stderr, "Found incomplete session %s (objective: %s, iteration: %d)\n",
			incompleteSession.SessionID, incompleteSession.ObjectiveID, incompleteSession.Iteration)
		crashedSession = incompleteSession
		// Don't mark as crashed yet - wait for HQ to decide whether to resume
	}

	// Check for unsynced activity from previous run
	unsyncedEvents, err := localDB.GetUnsyncedActivity(1000)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to check for unsynced activity: %v\n", err)
	} else if len(unsyncedEvents) > 0 {
		fmt.Fprintf(os.Stderr, "Found %d unsynced activity events from previous run\n", len(unsyncedEvents))
		runner.pendingRecoveryEvents = unsyncedEvents
	}

	// Store crashed session for potential resumption
	if crashedSession != nil {
		runner.crashedSession = crashedSession
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

// Heartbeat configuration
const (
	heartbeatInterval = 10 * time.Second
)

// workerRunner handles the main worker loop.
type workerRunner struct {
	conn        *worker.Conn
	receiver    *worker.Receiver
	identity    *crypto.WorkerIdentity
	localDB     *worker.LocalDB
	hqPublicKey string
	dataDir     string

	// Components for execution
	promptLoader   *worker.WorkerPromptLoader
	projectManager *worker.ProjectManager

	// Worker state
	startedAt time.Time

	// Recovery state
	pendingRecoveryEvents []*worker.ActivityEvent
	crashedSession        *worker.SessionState

	// Current execution state
	mu               sync.Mutex
	currentObjective *worker.ObjectivePayload
	currentSession   *worker.WorkerSession
	currentSessionID string
	currentCancel    context.CancelFunc
}

// run executes the main worker loop.
func (r *workerRunner) run(ctx context.Context) error {
	// Start heartbeat goroutine
	go r.heartbeatLoop(ctx)

	// Recover unsynced activity from previous run
	if len(r.pendingRecoveryEvents) > 0 {
		r.recoverActivity()
	}

	// Report crashed session to HQ and request resumption
	if r.crashedSession != nil {
		r.reportCrashedSession()
	}

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

// heartbeatLoop sends periodic heartbeats to HQ.
func (r *workerRunner) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.sendHeartbeat()
		}
	}
}

// reportCrashedSession sends a crash report to HQ for a session that didn't complete.
func (r *workerRunner) reportCrashedSession() {
	session := r.crashedSession
	if session == nil {
		return
	}

	fmt.Fprintf(os.Stderr, "Reporting crashed session %s to HQ...\n", session.SessionID)

	// Send crash report
	report := &worker.CrashReportPayload{
		WorkerID:     r.identity.ID,
		ObjectiveID:  session.ObjectiveID,
		SessionID:    session.SessionID,
		Hat:          session.Hat,
		Iteration:    session.Iteration,
		TokensInput:  session.TokensInput,
		TokensOutput: session.TokensOutput,
		WorkDir:      session.WorkDir,
		CrashedAt:    time.Now(),
		CanResume:    session.Conversation != "" && session.Conversation != "[]",
	}

	if err := r.conn.SendCrashReport(report); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to send crash report: %v\n", err)
		// Mark as crashed locally since we couldn't report
		_ = r.localDB.MarkSessionComplete(session.SessionID, "crashed")
		r.crashedSession = nil
		return
	}

	fmt.Fprintf(os.Stderr, "Crash report sent, waiting for HQ decision...\n")
	// Note: HQ will either send a Resume message or a new Dispatch
	// The crashed session state is kept until then
}

// recoverActivity sends unsynced activity from previous runs to HQ.
func (r *workerRunner) recoverActivity() {
	events := r.pendingRecoveryEvents
	r.pendingRecoveryEvents = nil

	if len(events) == 0 {
		return
	}

	fmt.Fprintf(os.Stderr, "Recovering %d unsynced activity events...\n", len(events))

	// Group events by objective
	byObjective := make(map[string][]*worker.ActivityEvent)
	for _, event := range events {
		byObjective[event.ObjectiveID] = append(byObjective[event.ObjectiveID], event)
	}

	// Send each objective's events
	for objectiveID, objEvents := range byObjective {
		sessionID := ""
		if len(objEvents) > 0 {
			sessionID = objEvents[0].SessionID
		}

		if err := r.conn.SendActivity(objectiveID, sessionID, objEvents); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to send recovered activity for %s: %v\n", objectiveID, err)
			// Don't mark as synced if send failed
			continue
		}

		// Mark as synced in local DB
		ids := make([]string, len(objEvents))
		for i, e := range objEvents {
			ids[i] = e.ID
		}
		if err := r.localDB.MarkActivitySynced(ids); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to mark activity as synced: %v\n", err)
		}
	}

	fmt.Fprintf(os.Stderr, "Activity recovery complete\n")
}

// sendHeartbeat sends a heartbeat message with current worker state.
func (r *workerRunner) sendHeartbeat() {
	r.mu.Lock()
	state := worker.WorkerStateIdle
	objectiveID := ""
	sessionID := ""
	iteration := 0
	tokensInput := 0
	tokensOutput := 0

	if r.currentObjective != nil {
		state = worker.WorkerStateRunning
		objectiveID = r.currentObjective.Objective.ID
		sessionID = r.currentSessionID
		if r.currentSession != nil {
			iteration = r.currentSession.GetIteration()
			input, output := r.currentSession.GetTokenUsage()
			tokensInput = int(input)
			tokensOutput = int(output)
		}
	}
	r.mu.Unlock()

	uptime := int64(time.Since(r.startedAt).Seconds())

	_ = r.conn.SendHeartbeat(&worker.HeartbeatPayload{
		WorkerID:     r.identity.ID,
		State:        state,
		ObjectiveID:  objectiveID,
		SessionID:    sessionID,
		Iteration:    iteration,
		TokensInput:  tokensInput,
		TokensOutput: tokensOutput,
		Uptime:       uptime,
	})
}

// handleMessage processes a message from HQ.
func (r *workerRunner) handleMessage(ctx context.Context, msg *worker.Message) error {
	switch msg.Type {
	case worker.MsgTypeDispatch:
		return r.handleDispatch(ctx, msg)
	case worker.MsgTypeResume:
		return r.handleResume(ctx, msg)
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

// handleDispatch handles a dispatch message and executes the objective.
func (r *workerRunner) handleDispatch(ctx context.Context, msg *worker.Message) error {
	// 1. Parse dispatch payload
	payload, err := worker.ParsePayload[worker.DispatchPayload](msg)
	if err != nil {
		return fmt.Errorf("failed to parse dispatch payload: %w", err)
	}

	objective := payload.Objective
	r.mu.Lock()
	r.currentObjective = objective
	r.mu.Unlock()

	fmt.Fprintf(os.Stderr, "Received objective: %s\n", objective.Objective.Title)
	fmt.Fprintf(os.Stderr, "  ID: %s\n", objective.Objective.ID)
	fmt.Fprintf(os.Stderr, "  Hat: %s\n", objective.Objective.Hat)

	// 2. Decrypt secrets
	secrets, err := r.receiver.DecryptPayload(objective)
	if err != nil {
		return fmt.Errorf("failed to decrypt secrets: %w", err)
	}

	fmt.Fprintf(os.Stderr, "  Secrets decrypted: anthropic_key=%v, github_token=%v\n",
		secrets.AnthropicKey != "", secrets.GitHubToken != "")

	// 3. Store objective in local DB
	if err := r.localDB.StoreObjective(objective); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to store objective locally: %v\n", err)
	}

	// 4. Generate session ID
	sessionID := fmt.Sprintf("sess-%s", uuid.New().String()[:8])
	r.mu.Lock()
	r.currentSessionID = sessionID
	r.mu.Unlock()

	// 5. Send accepted message
	if err := r.conn.SendAccepted(objective.Objective.ID, sessionID); err != nil {
		return fmt.Errorf("failed to send accepted: %w", err)
	}

	// 6. Setup project
	fmt.Fprintf(os.Stderr, "Setting up project %s/%s...\n", objective.Project.GitHubOwner, objective.Project.GitHubRepo)

	// Use authenticated clone URL if we have a token
	cloneURL := objective.Project.CloneURL
	if secrets.GitHubToken != "" {
		cloneURL = worker.SetupAuthenticatedCloneURL(cloneURL, secrets.GitHubToken)
	}

	// Temporarily update the project with authenticated URL
	projectWithAuth := objective.Project
	projectWithAuth.CloneURL = cloneURL

	workDir, err := r.projectManager.SetupProject(projectWithAuth, objective.Objective.BaseBranch)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to setup project: %v", err)
		fmt.Fprintf(os.Stderr, "  %s\n", errMsg)
		_ = r.conn.SendFailed(objective.Objective.ID, sessionID, errMsg, 0)
		r.clearCurrentExecution()
		return nil
	}
	fmt.Fprintf(os.Stderr, "  Project ready at %s\n", workDir)

	// 7. Create work branch if specified
	branchName := objective.Objective.BaseBranch
	if branchName == "" {
		branchName = fmt.Sprintf("dex/%s", objective.Objective.ID[:8])
	}
	if err := r.projectManager.CreateBranch(workDir, branchName); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to create branch %s: %v\n", branchName, err)
	}

	// 8. Create session
	session := worker.NewWorkerSession(sessionID, objective.Objective.ID, objective.Objective.Hat, workDir)
	if objective.Objective.TokenBudget > 0 {
		session.SetBudgets(objective.Objective.TokenBudget, 0, 0)
	}

	// 9. Create execution context with cancellation
	execCtx, cancel := context.WithCancel(ctx)
	r.mu.Lock()
	r.currentCancel = cancel
	r.currentSession = session
	r.mu.Unlock()

	// 10. Create Anthropic client
	anthropicClient := toolbelt.NewAnthropicClient(&toolbelt.AnthropicConfig{
		APIKey: secrets.AnthropicKey,
	})
	if anthropicClient == nil {
		errMsg := "Failed to create Anthropic client - no API key"
		fmt.Fprintf(os.Stderr, "  %s\n", errMsg)
		_ = r.conn.SendFailed(objective.Objective.ID, sessionID, errMsg, 0)
		cancel()
		r.clearCurrentExecution()
		return nil
	}

	// 11. Create activity recorder
	syncInterval := objective.Sync.ActivityIntervalSec
	if syncInterval <= 0 {
		syncInterval = 30
	}
	activityRecorder := worker.NewWorkerActivityRecorder(r.localDB, r.conn, session, syncInterval)
	go activityRecorder.StartSyncLoop(execCtx)

	// 12. Create tool executor
	executor := worker.NewWorkerToolExecutor(workDir, objective.Project.GitHubOwner, objective.Project.GitHubRepo, secrets.GitHubToken)

	// 13. Create and run the Ralph loop
	fmt.Fprintf(os.Stderr, "Starting Ralph loop for hat '%s'...\n", session.Hat)

	loop := worker.NewWorkerRalphLoop(
		session,
		anthropicClient,
		activityRecorder,
		r.conn,
		r.promptLoader,
		executor,
		&objective.Objective,
		&objective.Project,
		secrets.GitHubToken,
	)

	// Enable checkpointing for crash recovery
	loop.SetLocalDB(r.localDB)

	// Set progress callback for logging
	loop.SetProgressCallback(func(iteration int, inputTokens, outputTokens int64) {
		fmt.Fprintf(os.Stderr, "  Iteration %d complete (tokens: %d in, %d out)\n", iteration, inputTokens, outputTokens)
	})

	// Run the loop
	report, err := loop.Run(execCtx)

	// Stop activity sync
	activityRecorder.StopSyncLoop()

	// Final flush
	if flushErr := activityRecorder.Flush(); flushErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: final activity flush failed: %v\n", flushErr)
	}

	// 14. Send completion or failure
	if err != nil {
		if err == worker.ErrCancelled {
			fmt.Fprintf(os.Stderr, "Objective cancelled\n")
			_ = r.conn.Send(worker.MsgTypeCancelled, nil)
		} else {
			fmt.Fprintf(os.Stderr, "Objective failed: %v\n", err)
			_ = r.conn.SendFailed(objective.Objective.ID, sessionID, err.Error(), session.GetIteration())
		}
	} else {
		fmt.Fprintf(os.Stderr, "Objective completed: %s\n", report.Status)
		fmt.Fprintf(os.Stderr, "  Summary: %s\n", report.Summary)
		fmt.Fprintf(os.Stderr, "  Iterations: %d, Tokens: %d\n", report.Iterations, report.TotalTokens)

		// Ensure completion time is set
		if report.CompletedAt.IsZero() {
			report.CompletedAt = time.Now()
		}

		if err := r.conn.SendCompleted(report); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to send completion: %v\n", err)
		}
	}

	// Update objective status in local DB
	status := "completed"
	if err != nil {
		if err == worker.ErrCancelled {
			status = "cancelled"
		} else {
			status = "failed"
		}
	}
	_ = r.localDB.UpdateObjectiveStatus(objective.Objective.ID, status)

	// Cleanup project directory to save disk space
	// Keep it around for a bit in case we need to debug
	if status == "completed" {
		// Only cleanup on successful completion
		// Failed/cancelled objectives might need debugging
		fmt.Fprintf(os.Stderr, "Cleaning up project directory: %s\n", workDir)
		if cleanupErr := r.projectManager.Cleanup(workDir); cleanupErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to cleanup project: %v\n", cleanupErr)
		}
	}

	cancel()
	r.clearCurrentExecution()

	return nil
}

// handleResume handles a resume message from HQ to continue a crashed session.
func (r *workerRunner) handleResume(ctx context.Context, msg *worker.Message) error {
	payload, err := worker.ParsePayload[worker.ResumePayload](msg)
	if err != nil {
		return fmt.Errorf("failed to parse resume payload: %w", err)
	}

	// Check if HQ approved the resumption
	if !payload.Approved {
		fmt.Fprintf(os.Stderr, "HQ declined resumption: %s\n", payload.Reason)
		// Mark the crashed session as failed
		if r.crashedSession != nil {
			_ = r.localDB.MarkSessionComplete(r.crashedSession.SessionID, "declined")
			r.crashedSession = nil
		}
		return nil
	}

	// Verify we have the crashed session
	if r.crashedSession == nil || r.crashedSession.SessionID != payload.SessionID {
		return fmt.Errorf("no matching crashed session for resumption: %s", payload.SessionID)
	}

	crashedSession := r.crashedSession
	r.crashedSession = nil

	fmt.Fprintf(os.Stderr, "Resuming session %s (objective: %s, iteration: %d)\n",
		crashedSession.SessionID, crashedSession.ObjectiveID, crashedSession.Iteration)

	// Decrypt secrets
	secrets, err := r.receiver.DecryptSecrets(payload.EncryptedSecrets)
	if err != nil {
		_ = r.localDB.MarkSessionComplete(crashedSession.SessionID, "decrypt_failed")
		return fmt.Errorf("failed to decrypt secrets for resumption: %w", err)
	}

	// Get the original objective from local DB
	objective, err := r.localDB.GetObjective(crashedSession.ObjectiveID)
	if err != nil || objective == nil {
		_ = r.localDB.MarkSessionComplete(crashedSession.SessionID, "objective_not_found")
		return fmt.Errorf("failed to get objective for resumption: %w", err)
	}

	// Verify work directory still exists
	if _, err := os.Stat(crashedSession.WorkDir); os.IsNotExist(err) {
		_ = r.localDB.MarkSessionComplete(crashedSession.SessionID, "workdir_missing")
		return fmt.Errorf("work directory no longer exists: %s", crashedSession.WorkDir)
	}

	// Create session with restored state
	session := worker.NewWorkerSession(
		crashedSession.SessionID,
		crashedSession.ObjectiveID,
		crashedSession.Hat,
		crashedSession.WorkDir,
	)

	// Set up execution context
	execCtx, cancel := context.WithCancel(ctx)
	r.mu.Lock()
	r.currentCancel = cancel
	r.currentSession = session
	r.currentSessionID = crashedSession.SessionID
	r.mu.Unlock()

	// Create Anthropic client
	anthropicClient := toolbelt.NewAnthropicClient(&toolbelt.AnthropicConfig{
		APIKey: secrets.AnthropicKey,
	})
	if anthropicClient == nil {
		cancel()
		_ = r.localDB.MarkSessionComplete(crashedSession.SessionID, "no_api_key")
		return fmt.Errorf("failed to create Anthropic client for resumption")
	}

	// Create activity recorder
	activityRecorder := worker.NewWorkerActivityRecorder(r.localDB, r.conn, session, 30)
	go activityRecorder.StartSyncLoop(execCtx)

	// Create tool executor
	executor := worker.NewWorkerToolExecutor(
		crashedSession.WorkDir,
		"", "", // GitHub owner/repo from objective if needed
		secrets.GitHubToken,
	)

	// Create Ralph loop
	loop := worker.NewWorkerRalphLoop(
		session,
		anthropicClient,
		activityRecorder,
		r.conn,
		r.promptLoader,
		executor,
		objective,
		&worker.Project{}, // Minimal project info
		secrets.GitHubToken,
	)
	loop.SetLocalDB(r.localDB)

	// Restore from checkpoint
	if err := loop.RestoreFromCheckpoint(crashedSession); err != nil {
		cancel()
		activityRecorder.StopSyncLoop()
		_ = r.localDB.MarkSessionComplete(crashedSession.SessionID, "restore_failed")
		return fmt.Errorf("failed to restore from checkpoint: %w", err)
	}

	// Send accepted message
	if err := r.conn.SendAccepted(crashedSession.ObjectiveID, crashedSession.SessionID); err != nil {
		cancel()
		activityRecorder.StopSyncLoop()
		return fmt.Errorf("failed to send accepted: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Resuming Ralph loop from iteration %d...\n", crashedSession.Iteration)

	// Run the loop
	report, err := loop.Run(execCtx)

	// Stop activity sync
	activityRecorder.StopSyncLoop()
	_ = activityRecorder.Flush()

	// Send completion or failure
	if err != nil {
		if err == worker.ErrCancelled {
			_ = r.conn.Send(worker.MsgTypeCancelled, nil)
		} else {
			_ = r.conn.SendFailed(crashedSession.ObjectiveID, crashedSession.SessionID, err.Error(), session.GetIteration())
		}
		_ = r.localDB.MarkSessionComplete(crashedSession.SessionID, "failed")
	} else {
		_ = r.conn.SendCompleted(report)
		_ = r.localDB.MarkSessionComplete(crashedSession.SessionID, "completed")
	}

	cancel()
	r.clearCurrentExecution()

	return nil
}

// handleCancel handles a cancel message.
func (r *workerRunner) handleCancel(ctx context.Context, msg *worker.Message) error {
	payload, err := worker.ParsePayload[worker.CancelPayload](msg)
	if err != nil {
		return fmt.Errorf("failed to parse cancel payload: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Cancelling objective: %s (reason: %s)\n", payload.ObjectiveID, payload.Reason)

	r.mu.Lock()
	cancel := r.currentCancel
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	return nil
}

// handlePing handles a ping message.
func (r *workerRunner) handlePing(ctx context.Context) error {
	r.mu.Lock()
	state := worker.WorkerStateIdle
	objectiveID := ""
	if r.currentObjective != nil {
		state = worker.WorkerStateRunning
		objectiveID = r.currentObjective.Objective.ID
	}
	r.mu.Unlock()

	return r.conn.SendPong(&worker.PongPayload{
		WorkerID:    r.identity.ID,
		State:       state,
		ObjectiveID: objectiveID,
	})
}

// handleShutdown handles a shutdown message.
func (r *workerRunner) handleShutdown(ctx context.Context) error {
	fmt.Fprintf(os.Stderr, "Shutdown requested\n")

	// Cancel any running execution
	r.mu.Lock()
	if r.currentCancel != nil {
		r.currentCancel()
	}
	r.mu.Unlock()

	// Send acknowledgment
	if err := r.conn.Send(worker.MsgTypeShutdownAck, nil); err != nil {
		return err
	}

	// Exit cleanly
	os.Exit(0)
	return nil
}

// clearCurrentExecution resets the current execution state.
func (r *workerRunner) clearCurrentExecution() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.currentObjective = nil
	r.currentSession = nil
	r.currentSessionID = ""
	r.currentCancel = nil
}
