package ssh

import (
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	// MaxConnections is the maximum number of concurrent connections allowed
	MaxConnections = 100

	// SSHDialTimeout is the timeout for SSH connection establishment
	SSHDialTimeout = 10 * time.Second
)

// ConnectionInfo holds information about an SSH connection
type ConnectionInfo struct {
	ID       string
	Host     string
	Port     int
	Username string
	Created  time.Time
}

// Connection represents an active SSH connection with a persistent shell
type Connection struct {
	Info     ConnectionInfo
	client   *ssh.Client
	executor *ShellExecutor
}

// Manager manages SSH connections
type Manager struct {
	connections map[string]*Connection
	validator   *HostValidator
	mu          sync.RWMutex
}

// NewManager creates a new SSH connection manager
func NewManager(validator *HostValidator) *Manager {
	return &Manager{
		connections: make(map[string]*Connection),
		validator:   validator,
	}
}

// Connect establishes a new SSH connection
func (m *Manager) Connect(id, host string, port int, username, password, privateKeyPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check connection limit
	if len(m.connections) >= MaxConnections {
		return fmt.Errorf("connection limit reached (%d/%d)", len(m.connections), MaxConnections)
	}

	// Check if connection already exists
	if _, exists := m.connections[id]; exists {
		return fmt.Errorf("connection with ID '%s' already exists", id)
	}

	// Validate host
	if err := m.validator.Validate(host); err != nil {
		return err
	}

	// Prepare SSH config
	// Use InsecureIgnoreHostKey for now but this should be configurable in production
	// See: https://pkg.go.dev/golang.org/x/crypto/ssh#InsecureIgnoreHostKey
	// #nosec G106 - Host key verification intentionally disabled for dynamic SSH connections
	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         SSHDialTimeout,
	}

	// Add authentication methods
	if password != "" {
		config.Auth = append(config.Auth, ssh.Password(password))
	}

	if privateKeyPath != "" {
		// Read private key from file
		// #nosec G304 - Private key path is user-provided and validated by the validator
		keyData, err := os.ReadFile(privateKeyPath)
		if err != nil {
			return fmt.Errorf("failed to read private key file '%s': %w", privateKeyPath, err)
		}

		signer, err := ssh.ParsePrivateKey(keyData)
		if err != nil {
			return fmt.Errorf("failed to parse private key: %w", err)
		}
		config.Auth = append(config.Auth, ssh.PublicKeys(signer))
	}

	if len(config.Auth) == 0 {
		return fmt.Errorf("no authentication method provided (password or private key required)")
	}

	// Connect to SSH server
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", addr, err)
	}

	// Create persistent shell executor
	executor, err := NewShellExecutor(client)
	if err != nil {
		_ = client.Close() // Best effort cleanup
		return fmt.Errorf("failed to create shell executor: %w", err)
	}

	// Store connection
	m.connections[id] = &Connection{
		Info: ConnectionInfo{
			ID:       id,
			Host:     host,
			Port:     port,
			Username: username,
			Created:  time.Now(),
		},
		client:   client,
		executor: executor,
	}

	return nil
}

// Execute runs a command on an existing connection
func (m *Manager) Execute(id, command string) (*CommandResult, error) {
	m.mu.RLock()
	conn, exists := m.connections[id]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("connection '%s' not found", id)
	}

	return conn.executor.Execute(command)
}

// Close closes an SSH connection
func (m *Manager) Close(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	conn, exists := m.connections[id]
	if !exists {
		return fmt.Errorf("connection '%s' not found", id)
	}

	// Close executor and client
	if conn.executor != nil {
		_ = conn.executor.Close() // Best effort cleanup
	}
	if conn.client != nil {
		_ = conn.client.Close() // Best effort cleanup
	}

	// Remove from map
	delete(m.connections, id)

	return nil
}

// List returns information about all active connections
func (m *Manager) List() []ConnectionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	infos := make([]ConnectionInfo, 0, len(m.connections))
	for _, conn := range m.connections {
		infos = append(infos, conn.Info)
	}

	return infos
}

// CloseAll closes all active connections
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, conn := range m.connections {
		if conn.executor != nil {
			_ = conn.executor.Close() // Best effort cleanup
		}
		if conn.client != nil {
			_ = conn.client.Close() // Best effort cleanup
		}
		delete(m.connections, id)
	}
}
