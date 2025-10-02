# MCP SSH Server

A Model Context Protocol (MCP) server for secure SSH connections to remote hosts with persistent shell sessions.

## Features

- 🔐 Secure SSH with password or key authentication
- 🔄 Persistent sessions - environment and directory state maintained
- 🛡️ Host validation with glob patterns
- 📊 Multiple simultaneous connections
- 🎯 Pure Go implementation

## Installation

**Prerequisites:** Go 1.25 or higher

```bash
go build -o mcp-ssh
```

## Usage

```bash
./mcp-ssh --allowed-hosts "192.168.1.*,*.example.com"
```

**Flags:**
- `--allowed-hosts` (required): Comma-separated host patterns
- `--log-level`: Log level (default: info)
- `--log-file`: Log file path (default: stderr)

## MCP Tools

### `ssh_connect`
Establishes SSH connection.

**Parameters:**
- `connection_id` (string): Unique identifier
- `host` (string): Remote host
- `port` (number): SSH port (default: 22)
- `username` (string): SSH username
- `password` (string): Password (optional)
- `private_key_path` (string): Private key path (optional)

### `ssh_execute`
Executes command on active connection. Environment persists between commands.

**Parameters:**
- `connection_id` (string): Connection identifier
- `command` (string): Command to execute

### `ssh_close`
Closes SSH connection.

**Parameters:**
- `connection_id` (string): Connection to close

### `ssh_list`
Lists all active connections.

## Claude Desktop Configuration

**macOS:** `~/Library/Application Support/Claude/claude_desktop_config.json`
**Windows:** `%APPDATA%\Claude\claude_desktop_config.json`

```json
{
  "mcpServers": {
    "ssh": {
      "command": "/path/to/mcp-ssh",
      "args": ["--allowed-hosts", "192.168.1.*,*.example.com"]
    }
  }
}
```

## Security

- ⚠️ **Host Key Verification:** Currently uses `InsecureIgnoreHostKey()`. Implement proper verification for production.
- 🔒 **Host Allowlist:** Always use `--allowed-hosts` to restrict access.
- 🔑 **Credentials:** Handled in memory only, never logged.

## Development

```bash
# Run tests
go test ./...

# Cross-compile
GOOS=linux GOARCH=amd64 go build -o mcp-ssh-linux
GOOS=darwin GOARCH=amd64 go build -o mcp-ssh-macos
GOOS=windows GOARCH=amd64 go build -o mcp-ssh.exe
```

## License

MIT
