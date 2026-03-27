<p align="center">
  <img src="intentra-cli-logo.png" alt="Intentra CLI" width="400">
</p>

# Overview

Open-source monitoring tool for AI coding assistants. Captures events from Cursor, Claude Code, Gemini CLI, GitHub Copilot, and Windsurf, normalizes them into a unified schema, and aggregates them into scans.

**Local-first by default** - all data stays on your machine. For advanced observability and team features, connect to [intentra.sh](https://intentra.sh).

## Installation

**macOS/Linux:**

```bash
curl -fsSL https://install.intentra.sh | sh
```

**Homebrew:**

```bash
brew install intentrahq/intentra/intentra
```

**Windows (PowerShell):**

```powershell
irm https://install.intentra.sh/install.ps1 | iex
```

Verify installation:

```bash
intentra --version
```

## Quick Start

```bash
intentra install
intentra hooks status
intentra scan list
```

All scans are stored locally at `~/.intentra/scans/`.

## Commands

| Command | Description |
|---------|-------------|
| `intentra install [tool]` | Install hooks for AI tools (cursor, claude, gemini, copilot, windsurf, all) |
| `intentra uninstall [tool]` | Remove hooks from AI tools |
| `intentra hooks status` | Check hook installation status |
| `intentra login` | Authenticate with intentra.sh |
| `intentra logout` | Clear authentication |
| `intentra status` | Show authentication status |
| `intentra scan list` | List captured scans |
| `intentra scan show <id>` | Show scan details |
| `intentra scan today` | List today's scans |
| `intentra config show` | Display configuration |
| `intentra config init` | Generate sample config |
| `intentra config validate` | Validate configuration |

### Global Options

| Option | Description |
|--------|-------------|
| `--debug, -d` | Enable debug output (HTTP requests, local scan saves) |
| `--config, -c` | Config file path (default: ~/.intentra/config.yaml) |

## Supported Tools

| Tool | Status |
|------|--------|
| Cursor | Supported |
| Claude Code | Supported |
| GitHub Copilot | Supported |
| Windsurf | Supported |
| Gemini CLI | Supported |

## Event Normalization

The CLI normalizes tool-specific hook events into a unified snake_case format. Each tool has its own normalizer in `internal/hooks/`:

```
Native Event → normalizer_<tool>.go → NormalizedType (snake_case)
```

Key normalized event types:
- `before_prompt` / `after_response` - Prompt-response cycle
- `before_tool` / `after_tool` - Generic tool execution
- `before_file_edit` / `after_file_edit` - File operations
- `before_shell` / `after_shell` - Shell commands
- `stop` / `session_end` - Scan boundaries

See `internal/hooks/normalizer.go` for the full list of normalized types.

## Debug Mode

Enable debug mode to see HTTP requests and save scans locally:

**Using the -d flag:**
```bash
intentra -d status
intentra -d scan list
```

**Using config (persists for hooks):**
```yaml
# ~/.intentra/config.yaml
debug: true
```

When debug mode is enabled:
- HTTP requests are logged with status codes: `[DEBUG] POST https://api.intentra.sh/scans -> 200`
- Scans are saved locally to `~/.intentra/scans/` regardless of sync status

Note: Using `-d` automatically sets `debug: true` in the config file.

## Local Storage

Scans and data are stored in `~/.intentra/`:

| Path | Description |
|------|-------------|
| `~/.intentra/scans/` | Locally saved scans (when debug enabled) |
| `~/.intentra/config.yaml` | Configuration file |
| `~/.intentra/credentials.json` | Auth credentials (after `intentra login`) |

## Configuration

Configuration file location: `~/.intentra/config.yaml`

### Local-Only Mode (Default)

```yaml
server:
  enabled: false
```

### Server Mode (Advanced Observability)

Connect to [intentra.sh](https://intentra.sh) for dashboards, team analytics, and centralized monitoring.

**Recommended: Use `intentra login`**
```bash
intentra login
```

This uses OAuth to authenticate your device and automatically syncs data.

**Enterprise: API Key Authentication**

For programmatic access, Enterprise organizations can generate API keys in Settings > API Keys:

```yaml
server:
  enabled: true
  endpoint: "https://api.intentra.sh"
  auth:
    mode: "api_key"
    api_key:
      key_id: "apk_..."
      secret: "intentra_sk_..."
```

### Rich Traces

Enable detailed tool call capture for the [Session Deep Dive](https://intentra.sh/docs/guides/concepts#session-deep-dive) feature:

```bash
export INTENTRA_RICH_TRACES=true
```

When enabled, tool call inputs and outputs are captured alongside standard event data. Content is automatically redacted for secrets and truncated to 10KB per field. Requires organization-level enablement in Intentra dashboard settings.

## Documentation

Full documentation: [docs.intentra.sh](https://docs.intentra.sh)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

[Apache License 2.0](LICENSE)
