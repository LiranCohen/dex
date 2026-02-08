// Package daemon provides utilities for running as a background daemon.
package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

// PIDFile manages a PID file for daemon processes.
type PIDFile struct {
	Path string
}

// NewPIDFile creates a new PIDFile manager.
func NewPIDFile(dir, name string) *PIDFile {
	return &PIDFile{
		Path: filepath.Join(dir, name+".pid"),
	}
}

// Write writes the current process PID to the file.
func (p *PIDFile) Write() error {
	return os.WriteFile(p.Path, []byte(strconv.Itoa(os.Getpid())), 0644)
}

// Read reads the PID from the file.
func (p *PIDFile) Read() (int, error) {
	data, err := os.ReadFile(p.Path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(string(data))
}

// Remove removes the PID file.
func (p *PIDFile) Remove() error {
	return os.Remove(p.Path)
}

// IsRunning checks if the process with the stored PID is still running.
func (p *PIDFile) IsRunning() bool {
	pid, err := p.Read()
	if err != nil {
		return false
	}
	return IsProcessRunning(pid)
}

// IsProcessRunning checks if a process with the given PID is running.
func IsProcessRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds. Send signal 0 to check if process exists.
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

// StopProcess sends SIGTERM to the process and waits for it to exit.
func (p *PIDFile) StopProcess() error {
	pid, err := p.Read()
	if err != nil {
		return fmt.Errorf("failed to read PID file: %w", err)
	}

	if !IsProcessRunning(pid) {
		// Process already stopped, clean up PID file
		_ = p.Remove()
		return nil
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	// Send SIGTERM
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM: %w", err)
	}

	// Note: We don't wait here - the caller should poll IsRunning() if needed
	return nil
}

// Daemonize forks the current process to run in the background.
// Returns true in the parent (which should exit), false in the child.
// The child process will have the given args.
func Daemonize(args []string) (bool, error) {
	// We're the parent - need to fork
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = os.Environ()

	// Detach from terminal
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // Create new session
	}

	if err := cmd.Start(); err != nil {
		return false, fmt.Errorf("failed to start daemon: %w", err)
	}

	// Parent should exit
	return true, nil
}

// IsDaemonized checks if the current process is running as a daemon.
// This is detected by checking if we're a session leader.
func IsDaemonized() bool {
	// If we're the session leader, we're daemonized
	sid, err := syscall.Getsid(0)
	if err != nil {
		return false
	}
	return os.Getpid() == sid
}
