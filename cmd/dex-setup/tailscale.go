package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"os/user"
	"regexp"
	"strings"
	"time"
)

var (
	ErrTailscaleNotInstalled  = errors.New("tailscale is not installed")
	ErrTailscaleNotRunning    = errors.New("tailscale daemon is not running")
	ErrTailscaleNotConnected  = errors.New("tailscale is not connected")
	ErrTailscaleNoAuthURL     = errors.New("could not get auth URL")
	ErrTailscaleServeNotReady = errors.New("tailscale serve not configured")
)

// TailscaleStatus represents the status of Tailscale
type TailscaleStatus struct {
	BackendState   string         `json:"BackendState"`
	Self           SelfInfo       `json:"Self"`
	CurrentTailnet *TailnetInfo   `json:"CurrentTailnet,omitempty"`
}

// TailnetInfo contains information about the current tailnet
type TailnetInfo struct {
	Name           string `json:"Name"`
	MagicDNSSuffix string `json:"MagicDNSSuffix"`
}

// SelfInfo contains information about the current node
type SelfInfo struct {
	DNSName       string   `json:"DNSName"`
	TailscaleIPs  []string `json:"TailscaleIPs"`
	HostName      string   `json:"HostName"`
	Online        bool     `json:"Online"`
	ExitNodeIP    string   `json:"ExitNodeIP,omitempty"`
}

// TailscaleServeStatus represents the serve configuration status
type TailscaleServeStatus struct {
	TCP         map[string]any  `json:"TCP"`
	Web         map[string]any  `json:"Web"`
	AllowFunnel map[string]bool `json:"AllowFunnel"`
}

// CheckTailscaleInstalled checks if tailscale CLI is available
func CheckTailscaleInstalled() error {
	_, err := exec.LookPath("tailscale")
	if err != nil {
		return ErrTailscaleNotInstalled
	}
	return nil
}

// GetTailscaleStatus returns the current Tailscale status
func GetTailscaleStatus() (*TailscaleStatus, error) {
	if err := CheckTailscaleInstalled(); err != nil {
		return nil, err
	}

	cmd := exec.Command("tailscale", "status", "--json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	var status TailscaleStatus
	if err := json.Unmarshal(output, &status); err != nil {
		return nil, fmt.Errorf("failed to parse status: %w", err)
	}

	return &status, nil
}

// IsTailscaleConnected checks if Tailscale is connected to a tailnet
func IsTailscaleConnected() (bool, error) {
	status, err := GetTailscaleStatus()
	if err != nil {
		return false, err
	}
	return status.BackendState == "Running", nil
}

// GetTailscaleAuthURL initiates login and returns the auth URL
// This starts the auth process and captures the URL from output
func GetTailscaleAuthURL(hostname string) (string, error) {
	if err := CheckTailscaleInstalled(); err != nil {
		return "", err
	}

	// Get current user for operator flag
	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("failed to get current user: %w", err)
	}

	// Build command args
	args := []string{"up", "--reset"}
	if hostname != "" {
		args = append(args, "--hostname="+hostname)
	}
	args = append(args, "--operator="+currentUser.Username)

	cmd := exec.Command("tailscale", args...)

	// Capture stderr where auth URL is written
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Start the command (don't wait for it to complete)
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start tailscale up: %w", err)
	}

	// Give it a moment to produce output
	time.Sleep(2 * time.Second)

	// Look for auth URL in output
	output := stderr.String()
	authURL := extractAuthURL(output)
	if authURL == "" {
		// Check if already authenticated
		connected, _ := IsTailscaleConnected()
		if connected {
			return "", nil // Already connected, no URL needed
		}
		return "", ErrTailscaleNoAuthURL
	}

	return authURL, nil
}

// StartTailscaleAuth starts the authentication process and returns auth URL
// This is a non-blocking version that returns immediately with the URL
func StartTailscaleAuth(hostname string) (authURL string, checkConnected func() bool, err error) {
	if err := CheckTailscaleInstalled(); err != nil {
		return "", nil, err
	}

	// Check if already connected
	connected, _ := IsTailscaleConnected()
	if connected {
		return "", func() bool { return true }, nil
	}

	// Get current user
	currentUser, err := user.Current()
	if err != nil {
		return "", nil, fmt.Errorf("failed to get current user: %w", err)
	}

	// Build args
	args := []string{"up", "--reset"}
	if hostname != "" {
		args = append(args, "--hostname="+hostname)
	}
	args = append(args, "--operator="+currentUser.Username)

	cmd := exec.Command("tailscale", args...)

	// Capture both stdout and stderr - tailscale may output URL to either
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", nil, fmt.Errorf("failed to start tailscale up: %w", err)
	}

	// Read output in background to find auth URL from either pipe
	urlChan := make(chan string, 1)
	scanPipe := func(pipe io.Reader) {
		scanner := bufio.NewScanner(pipe)
		for scanner.Scan() {
			line := scanner.Text()
			if url := extractAuthURL(line); url != "" {
				select {
				case urlChan <- url:
				default:
				}
				return
			}
		}
	}
	go scanPipe(stdoutPipe)
	go scanPipe(stderrPipe)

	// Goroutine to wait on the command and prevent zombies
	go func() {
		cmd.Wait()
	}()

	// Wait for URL with timeout
	select {
	case url := <-urlChan:
		if url == "" {
			// May already be connected
			if conn, _ := IsTailscaleConnected(); conn {
				return "", func() bool { return true }, nil
			}
			return "", nil, ErrTailscaleNoAuthURL
		}
		checkFn := func() bool {
			conn, _ := IsTailscaleConnected()
			return conn
		}
		return url, checkFn, nil
	case <-time.After(10 * time.Second):
		// Check if already connected
		if conn, _ := IsTailscaleConnected(); conn {
			return "", func() bool { return true }, nil
		}
		return "", nil, ErrTailscaleNoAuthURL
	}
}

// extractAuthURL extracts the auth URL from tailscale output
func extractAuthURL(output string) string {
	// Match various Tailscale auth URL patterns
	patterns := []string{
		`https://login\.tailscale\.com/[^\s]+`,
		`https://[a-z]+\.tailscale\.com/[^\s]*auth[^\s]*`,
	}
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if match := re.FindString(output); match != "" {
			return match
		}
	}
	return ""
}

// GetTailscaleDNSName returns the DNS name for the current node
func GetTailscaleDNSName() (string, error) {
	status, err := GetTailscaleStatus()
	if err != nil {
		return "", err
	}

	if status.BackendState != "Running" {
		return "", ErrTailscaleNotConnected
	}

	// DNS name typically has a trailing dot, remove it
	dnsName := strings.TrimSuffix(status.Self.DNSName, ".")
	if dnsName == "" {
		return "", errors.New("no DNS name available")
	}

	return dnsName, nil
}

// GetTailscaleServeURL returns the HTTPS URL for tailscale serve
func GetTailscaleServeURL(port int) (string, error) {
	dnsName, err := GetTailscaleDNSName()
	if err != nil {
		return "", err
	}

	// The URL will be https://<dns-name> when using default HTTPS port (443)
	if port == 443 || port == 0 {
		return fmt.Sprintf("https://%s", dnsName), nil
	}

	return fmt.Sprintf("https://%s:%d", dnsName, port), nil
}

// ConfigureTailscaleServe sets up tailscale serve to proxy to a local port
func ConfigureTailscaleServe(localPort int, httpsPort int) error {
	if err := CheckTailscaleInstalled(); err != nil {
		return err
	}

	// Reset any existing serve config first
	resetCmd := exec.Command("tailscale", "serve", "reset")
	resetCmd.Run() // Ignore errors, might not have existing config

	// Configure serve
	// tailscale serve --bg --https=443 http://127.0.0.1:8080
	args := []string{"serve", "--bg"}
	if httpsPort != 0 && httpsPort != 443 {
		args = append(args, fmt.Sprintf("--https=%d", httpsPort))
	} else {
		args = append(args, "--https=443")
	}
	args = append(args, fmt.Sprintf("http://127.0.0.1:%d", localPort))

	cmd := exec.Command("tailscale", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to configure tailscale serve: %w (output: %s)", err, string(output))
	}

	return nil
}

// ResetTailscaleServe removes all tailscale serve configuration
func ResetTailscaleServe() error {
	if err := CheckTailscaleInstalled(); err != nil {
		return err
	}

	cmd := exec.Command("tailscale", "serve", "reset")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to reset tailscale serve: %w", err)
	}

	return nil
}

// WaitForTailscaleConnection polls until Tailscale is connected or timeout
func WaitForTailscaleConnection(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		connected, err := IsTailscaleConnected()
		if err == nil && connected {
			return nil
		}
		time.Sleep(2 * time.Second)
	}

	return ErrTailscaleNotConnected
}
