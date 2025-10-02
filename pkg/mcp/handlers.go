package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/denysvitali/mcp-ssh/pkg/ssh"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/sirupsen/logrus"
)

// Handlers manages MCP tool handlers for SSH operations
type Handlers struct {
	manager *ssh.Manager
	logger  *logrus.Logger
}

// NewHandlers creates a new handlers instance
func NewHandlers(manager *ssh.Manager, logger *logrus.Logger) *Handlers {
	if manager == nil {
		panic("ssh.Manager cannot be nil")
	}
	if logger == nil {
		panic("logger cannot be nil")
	}
	return &Handlers{
		manager: manager,
		logger:  logger,
	}
}

// validateConnectionID validates the connection ID format
func validateConnectionID(id string) error {
	if id == "" {
		return fmt.Errorf("connection_id cannot be empty")
	}
	if len(id) > 128 {
		return fmt.Errorf("connection_id too long (max 128 characters)")
	}
	// Only allow alphanumeric, dash, and underscore
	for _, r := range id {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_') {
			return fmt.Errorf("connection_id contains invalid characters (only alphanumeric, dash, underscore allowed)")
		}
	}
	return nil
}

// validatePort validates the port number
func validatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", port)
	}
	return nil
}

// validateCommand validates the command string
func validateCommand(cmd string) error {
	if cmd == "" {
		return fmt.Errorf("command cannot be empty")
	}
	if len(cmd) > 1048576 { // 1MB limit
		return fmt.Errorf("command too long (max 1MB)")
	}
	return nil
}

// validateAuthMethod validates authentication method is provided
func validateAuthMethod(password, privateKeyPath string) error {
	if password == "" && privateKeyPath == "" {
		return fmt.Errorf("either 'password' or 'private_key_path' must be provided")
	}
	return nil
}

// HandleConnect handles the ssh_connect tool
func (h *Handlers) HandleConnect(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Extract parameters
	connectionID, err := req.RequireString("connection_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Validate connection ID
	if err := validateConnectionID(connectionID); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	host, err := req.RequireString("host")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Validate host is not empty after trim
	if strings.TrimSpace(host) == "" {
		return mcp.NewToolResultError("host cannot be empty"), nil
	}

	username, err := req.RequireString("username")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Validate username is not empty after trim
	if strings.TrimSpace(username) == "" {
		return mcp.NewToolResultError("username cannot be empty"), nil
	}

	// Optional parameters
	port := int(req.GetFloat("port", 22))
	if err := validatePort(port); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	password := req.GetString("password", "")
	privateKeyPath := req.GetString("private_key_path", "")

	// Validate authentication method
	if err := validateAuthMethod(password, privateKeyPath); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	h.logger.WithFields(logrus.Fields{
		"connection_id": connectionID,
		"host":          host,
		"port":          port,
		"username":      username,
	}).Info("Attempting SSH connection")

	// Establish connection
	if err := h.manager.Connect(connectionID, host, port, username, password, privateKeyPath); err != nil {
		h.logger.WithError(err).Error("Failed to establish SSH connection")
		return mcp.NewToolResultError(fmt.Sprintf("Failed to connect: %v", err)), nil
	}

	h.logger.Info("SSH connection established successfully")

	// Return success response
	response := map[string]interface{}{
		"success":       true,
		"connection_id": connectionID,
		"host":          host,
		"port":          port,
		"username":      username,
		"message":       "SSH connection established successfully",
	}

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		h.logger.WithError(err).Error("Failed to marshal response")
		return mcp.NewToolResultError(fmt.Sprintf("Internal error: failed to marshal response: %v", err)), nil
	}
	return mcp.NewToolResultText(string(jsonResponse)), nil
}

// HandleExecute handles the ssh_execute tool
func (h *Handlers) HandleExecute(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Extract parameters
	connectionID, err := req.RequireString("connection_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Validate connection ID
	if err := validateConnectionID(connectionID); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	command, err := req.RequireString("command")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Validate command
	if err := validateCommand(command); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	h.logger.WithFields(logrus.Fields{
		"connection_id": connectionID,
		"command":       command,
	}).Debug("Executing SSH command")

	// Execute command
	result, err := h.manager.Execute(connectionID, command)
	if err != nil {
		h.logger.WithError(err).Error("Failed to execute SSH command")
		return mcp.NewToolResultError(fmt.Sprintf("Failed to execute command: %v", err)), nil
	}

	h.logger.WithFields(logrus.Fields{
		"exit_code": result.ExitCode,
	}).Debug("Command executed successfully")

	// Return result
	response := map[string]interface{}{
		"success":   true,
		"stdout":    result.Stdout,
		"stderr":    result.Stderr,
		"exit_code": result.ExitCode,
	}

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		h.logger.WithError(err).Error("Failed to marshal response")
		return mcp.NewToolResultError(fmt.Sprintf("Internal error: failed to marshal response: %v", err)), nil
	}
	return mcp.NewToolResultText(string(jsonResponse)), nil
}

// HandleClose handles the ssh_close tool
func (h *Handlers) HandleClose(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Extract parameters
	connectionID, err := req.RequireString("connection_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Validate connection ID
	if err := validateConnectionID(connectionID); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	h.logger.WithFields(logrus.Fields{
		"connection_id": connectionID,
	}).Info("Closing SSH connection")

	// Close connection
	if err := h.manager.Close(connectionID); err != nil {
		h.logger.WithError(err).Error("Failed to close SSH connection")
		return mcp.NewToolResultError(fmt.Sprintf("Failed to close connection: %v", err)), nil
	}

	h.logger.Info("SSH connection closed successfully")

	// Return success response
	response := map[string]interface{}{
		"success":       true,
		"connection_id": connectionID,
		"message":       "SSH connection closed successfully",
	}

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		h.logger.WithError(err).Error("Failed to marshal response")
		return mcp.NewToolResultError(fmt.Sprintf("Internal error: failed to marshal response: %v", err)), nil
	}
	return mcp.NewToolResultText(string(jsonResponse)), nil
}

// HandleList handles the ssh_list tool
func (h *Handlers) HandleList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	h.logger.Debug("Listing active SSH connections")

	// Get list of connections
	connections := h.manager.List()

	h.logger.WithFields(logrus.Fields{
		"count": len(connections),
	}).Debug("Retrieved connection list")

	// Convert to response format
	connList := make([]map[string]interface{}, len(connections))
	for i, conn := range connections {
		connList[i] = map[string]interface{}{
			"connection_id": conn.ID,
			"host":          conn.Host,
			"port":          conn.Port,
			"username":      conn.Username,
			"created":       conn.Created.Format("2006-01-02 15:04:05"),
		}
	}

	response := map[string]interface{}{
		"success":     true,
		"connections": connList,
		"count":       len(connections),
	}

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		h.logger.WithError(err).Error("Failed to marshal response")
		return mcp.NewToolResultError(fmt.Sprintf("Internal error: failed to marshal response: %v", err)), nil
	}
	return mcp.NewToolResultText(string(jsonResponse)), nil
}
