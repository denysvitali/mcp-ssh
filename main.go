package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/denysvitali/mcp-ssh/cmd"
	"github.com/denysvitali/mcp-ssh/pkg/mcp"
	"github.com/denysvitali/mcp-ssh/pkg/ssh"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sirupsen/logrus"
)

var (
	Version = "dev"
)

func main() {
	// Set up the server function
	cmd.ServerFunc = runServer

	// Execute cobra command to parse flags
	cmd.Execute()
}

func runServer() error {
	// Setup logger
	logger, logCleanup, err := cmd.SetupLogger()
	if err != nil {
		return fmt.Errorf("failed to setup logger: %w", err)
	}
	defer func() {
		if err := logCleanup(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close log file: %v\n", err)
		}
	}()

	logger.Info("Starting MCP SSH Server")

	// Get allowed hosts
	allowedHosts := cmd.GetAllowedHosts()
	if allowedHosts == "" {
		return fmt.Errorf("--allowed-hosts flag is required")
	}

	// Create host validator
	validator, err := ssh.NewHostValidator(allowedHosts)
	if err != nil {
		return fmt.Errorf("failed to create host validator: %w", err)
	}

	logger.WithFields(logrus.Fields{
		"allowed_hosts": allowedHosts,
	}).Info("Host validator initialized")

	// Create SSH manager
	sshManager := ssh.NewManager(validator)

	// Create MCP handlers
	handlers := mcp.NewHandlers(sshManager, logger)

	// Create MCP server
	mcpServer := server.NewMCPServer(
		"mcp-ssh",
		Version,
		server.WithToolCapabilities(true),
		server.WithLogging(),
		server.WithRecovery(),
	)

	// Define ssh_connect tool
	connectTool := mcpgo.NewTool(
		"ssh_connect",
		mcpgo.WithDescription("Establish an SSH connection to a remote host"),
		mcpgo.WithString("connection_id",
			mcpgo.Required(),
			mcpgo.Description("Unique identifier for this connection"),
		),
		mcpgo.WithString("host",
			mcpgo.Required(),
			mcpgo.Description("Remote host address (hostname or IP)"),
		),
		mcpgo.WithNumber("port",
			mcpgo.Description("SSH port (default: 22)"),
		),
		mcpgo.WithString("username",
			mcpgo.Required(),
			mcpgo.Description("SSH username"),
		),
		mcpgo.WithString("password",
			mcpgo.Description("SSH password (optional if using private_key_path)"),
		),
		mcpgo.WithString("private_key_path",
			mcpgo.Description("Path to SSH private key file (optional if using password)"),
		),
	)

	// Define ssh_execute tool
	executeTool := mcpgo.NewTool(
		"ssh_execute",
		mcpgo.WithDescription("Execute a command on an active SSH connection. Environment variables and working directory persist between commands."),
		mcpgo.WithString("connection_id",
			mcpgo.Required(),
			mcpgo.Description("Connection identifier"),
		),
		mcpgo.WithString("command",
			mcpgo.Required(),
			mcpgo.Description("Command to execute"),
		),
	)

	// Define ssh_close tool
	closeTool := mcpgo.NewTool(
		"ssh_close",
		mcpgo.WithDescription("Close an active SSH connection"),
		mcpgo.WithString("connection_id",
			mcpgo.Required(),
			mcpgo.Description("Connection identifier to close"),
		),
	)

	// Define ssh_list tool
	listTool := mcpgo.NewTool(
		"ssh_list",
		mcpgo.WithDescription("List all active SSH connections"),
	)

	// Add tools to server
	mcpServer.AddTool(connectTool, handlers.HandleConnect)
	mcpServer.AddTool(executeTool, handlers.HandleExecute)
	mcpServer.AddTool(closeTool, handlers.HandleClose)
	mcpServer.AddTool(listTool, handlers.HandleList)

	logger.Info("MCP tools registered")

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		logger.WithFields(logrus.Fields{
			"signal": sig.String(),
		}).Info("Received shutdown signal")

		// Close all SSH connections
		logger.Info("Closing all SSH connections")
		sshManager.CloseAll()

		cancel()
	}()

	// Start MCP server with stdio transport
	logger.Info("Starting MCP server on stdio transport")
	if err := server.ServeStdio(mcpServer); err != nil {
		logger.WithError(err).Error("Server error")
		return err
	}

	<-ctx.Done()
	logger.Info("MCP SSH Server stopped")
	return nil
}
