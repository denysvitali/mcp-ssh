package ssh

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	// Shell initialization timeouts
	shellInitialDrainDelay = 200 * time.Millisecond
	shellInitCommandDelay  = 100 * time.Millisecond

	// Command execution timeout
	defaultCommandTimeout = 30 * time.Second

	// Read timeouts
	stderrReadTimeout = 100 * time.Millisecond
	pollInterval      = 10 * time.Millisecond

	// Output size limits
	maxCommandSize = 1 * 1024 * 1024 // 1MB
	maxOutputSize  = 10 * 1024 * 1024 // 10MB
)

// CommandResult represents the result of a command execution
type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// ShellExecutor manages a persistent shell session for executing commands
type ShellExecutor struct {
	session *ssh.Session
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	stderr  *bufio.Reader
	mu      sync.Mutex
}

// NewShellExecutor creates a new persistent shell executor
func NewShellExecutor(client *ssh.Client) (*ShellExecutor, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Get stdin pipe
	stdin, err := session.StdinPipe()
	if err != nil {
		_ = session.Close() // Best effort cleanup
		return nil, fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	// Get stdout pipe
	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		_ = session.Close() // Best effort cleanup
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	// Get stderr pipe
	stderrPipe, err := session.StderrPipe()
	if err != nil {
		_ = session.Close() // Best effort cleanup
		return nil, fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	// Start shell
	if err := session.Shell(); err != nil {
		_ = session.Close() // Best effort cleanup
		return nil, fmt.Errorf("failed to start shell: %w", err)
	}

	executor := &ShellExecutor{
		session: session,
		stdin:   stdin,
		stdout:  bufio.NewReader(stdoutPipe),
		stderr:  bufio.NewReader(stderrPipe),
	}

	// Wait for initial shell output
	time.Sleep(shellInitialDrainDelay)
	executor.drainOutput()

	// Disable echo and set empty prompt for clean output
	initCommands := "stty -echo 2>/dev/null; export PS1=''\n"
	if _, err := stdin.Write([]byte(initCommands)); err != nil {
		_ = session.Close() // Best effort cleanup
		return nil, fmt.Errorf("failed to initialize shell: %w", err)
	}

	// Wait for init commands to complete and drain
	time.Sleep(shellInitCommandDelay)
	executor.drainOutput()

	return executor, nil
}

// Execute runs a command in the persistent shell and returns the result
func (e *ShellExecutor) Execute(command string) (*CommandResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Generate unique delimiter
	delimiter := fmt.Sprintf("__MCP_SSH_END_%d__", time.Now().UnixNano())

	// Validate command doesn't contain our delimiter pattern
	// This prevents command injection attacks where a malicious command
	// could fake the delimiter and manipulate exit codes
	if strings.Contains(command, "__MCP_SSH_END_") {
		return nil, fmt.Errorf("command contains forbidden delimiter pattern '__MCP_SSH_END_'")
	}

	// Prepare command with delimiter and exit code capture
	// We use a compound command that:
	// 1. Executes the user's command
	// 2. Captures the exit code
	// 3. Prints the delimiter followed by the exit code
	fullCommand := fmt.Sprintf(
		"%s\necho \"%s:$?\"\n",
		command,
		delimiter,
	)

	// Send command
	if _, err := e.stdin.Write([]byte(fullCommand)); err != nil {
		return nil, fmt.Errorf("failed to write command: %w", err)
	}

	// Read output until we see the delimiter
	var stdoutBuilder strings.Builder
	var stderrBuilder strings.Builder
	var exitCode int

	// Create channels for async reading
	stdoutChan := make(chan string, 1)
	stderrChan := make(chan string, 1)
	errChan := make(chan error, 2)

	// Read stdout
	go func() {
		output, code, err := e.readUntilDelimiter(e.stdout, delimiter)
		if err != nil {
			errChan <- err
			return
		}
		stdoutChan <- output
		exitCode = code
	}()

	// Read stderr
	go func() {
		output, err := e.readStderr(e.stderr, stderrReadTimeout)
		if err != nil {
			errChan <- err
			return
		}
		stderrChan <- output
	}()

	// Wait for both readers with timeout
	timeout := time.After(defaultCommandTimeout)

	var stdoutReceived, stderrReceived bool
	for !stdoutReceived || !stderrReceived {
		select {
		case stdout := <-stdoutChan:
			stdoutBuilder.WriteString(stdout)
			stdoutReceived = true
		case stderr := <-stderrChan:
			stderrBuilder.WriteString(stderr)
			stderrReceived = true
		case err := <-errChan:
			return nil, fmt.Errorf("read error: %w", err)
		case <-timeout:
			return nil, fmt.Errorf("command execution timed out")
		}
	}

	return &CommandResult{
		Stdout:   strings.TrimSpace(stdoutBuilder.String()),
		Stderr:   strings.TrimSpace(stderrBuilder.String()),
		ExitCode: exitCode,
	}, nil
}

// readUntilDelimiter reads from the reader until it finds the delimiter
func (e *ShellExecutor) readUntilDelimiter(reader *bufio.Reader, delimiter string) (string, int, error) {
	var output strings.Builder
	var exitCode int

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return output.String(), exitCode, nil
			}
			return "", 0, err
		}

		// Check if this line contains the delimiter
		if strings.Contains(line, delimiter) {
			// Extract exit code from delimiter line (format: __DELIMITER__:123)
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				_, _ = fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &exitCode)
			}
			return output.String(), exitCode, nil
		}

		output.WriteString(line)
	}
}

// readStderr reads stderr with a timeout (since stderr might not have data)
func (e *ShellExecutor) readStderr(reader *bufio.Reader, timeout time.Duration) (string, error) {
	var output strings.Builder

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		// Check if data is available
		if reader.Buffered() > 0 {
			line, err := reader.ReadString('\n')
			if err != nil && err != io.EOF {
				return "", err
			}
			output.WriteString(line)
			if err == io.EOF {
				break
			}
		} else {
			time.Sleep(pollInterval)
		}
	}

	return output.String(), nil
}

// drainOutput drains any pending output from stdout and stderr
func (e *ShellExecutor) drainOutput() {
	// Non-blocking drain
	for e.stdout.Buffered() > 0 {
		_, _ = e.stdout.ReadString('\n')
	}
	for e.stderr.Buffered() > 0 {
		_, _ = e.stderr.ReadString('\n')
	}
}

// Close closes the shell executor
func (e *ShellExecutor) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.stdin != nil {
		_ = e.stdin.Close() // Best effort cleanup
	}

	if e.session != nil {
		return e.session.Close()
	}

	return nil
}
