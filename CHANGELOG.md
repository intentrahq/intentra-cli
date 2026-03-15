# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.15.0] - 2026-03-14

### Added
- Gzip compression for all outgoing API requests (`SendScan` and `doJWTRequest`), reducing bandwidth usage
- `Content-Encoding: gzip` header set on compressed requests
- Flush failure tracking: queued scans that fail 10 consecutive times are automatically dropped
- `RecordFailure` function in `internal/queue` to track per-scan flush attempts via `.failures` sidecar files
- `Remove` now cleans up the failure counter file alongside the scan file

## [0.14.1] - 2026-03-14

### Fixed
- `toolArgs` string values are now properly JSON-encoded before assignment to `ToolInput`, preventing invalid JSON in scan payloads
- `sanitizeEvent` sets `ToolInput` and `ToolOutput` to `nil` instead of malformed redacted strings

### Changed
- `intentra install` uses `"intentra"` command name instead of resolved executable path for portable hook commands

### Removed
- Unused `ClaudeCodeHooks` and `ClaudeHookEntry` struct types from templates
- `Setup` hook type from Claude Code hook mappings (not a valid Claude Code lifecycle event)

## [0.14.0] - 2026-03-12

### Added
- `internal/queue` package: encrypted offline scan queue with AES-256-GCM; scans that fail to sync are persisted to `~/.intentra/queue/` and auto-flushed on login or next sync
- Queue enforces max 500 entries with FIFO eviction and 72-hour auto-expiry for stale entries
- `INTENTRA_NO_KEYCHAIN` environment variable to force file-based keyring backend (useful for CI/headless environments)
- `Description` field on OS keyring entries for better visibility in macOS Keychain and similar tools

### Changed
- `Encrypt`, `Decrypt`, `DeriveKey`, `ReadCacheKey` exported from `internal/auth` for cross-package use
- `DeriveKey` accepts salt and info parameters for domain separation between credential and queue encryption keys
- `intentra login` now flushes any offline-queued scans after successful authentication
- `intentra sync now` flushes offline queue in addition to regular scan sync
- `intentra sync now` warns on partial sync failure instead of returning a hard error
- Hook handler queues scans offline when sync fails and opportunistically flushes previous queue entries in background on successful sync

## [0.13.0] - 2026-03-05

### Added
- `internal/httputil` package consolidating `DefaultClient` (30s timeout) and `MaxResponseSize` (10 MB) used across auth, API, and hook packages
- `SendScanWithJWT` and `PatchSessionEnd` exported functions on `api` package, replacing private implementations in hook handler
- `GenerateScanID` and `SanitizePath` exported from `pkg/models` as single source of truth
- `NormalizedEventType` constants, `IsLLMCallEvent`, and `IsToolCallEvent` promoted to `pkg/models/event.go`
- `device.GetRawHardwareID()` for cross-package access to platform hardware identity
- `config.InvalidateCache()` for force-reloading config from disk
- `config.AuthModeAPIKey` constant replacing hardcoded `"api_key"` strings
- Generic `installJSONHookFile`, `uninstallJSONHookFile`, `installSettingsHookFile`, `uninstallSettingsHookFile` helpers replacing per-tool install/uninstall boilerplate
- JSON parse warning on corrupt hooks.json/settings.json files instead of silent overwrite

### Changed
- Config loading is now cached with mutex guard; subsequent `Load()` calls return cached result
- `EnsureDirectories` is now idempotent via `sync.Once`-style guard
- `SanitizePath` caches `os.UserHomeDir()` result via `sync.Once`
- `saveAPIConfig` uses typed `config.Load`/`config.SaveConfig` instead of raw YAML marshaling
- `cleanupStaleBuffers` moved from `ProcessEventWithEvent` to `handleStopEvent` (runs only when flushing)
- `deriveSessionKey` simplified: removed redundant nested `os.Stat` call
- `redactContent` and sanitize helpers use `strconv.Itoa` instead of `fmt.Sprintf` for lower allocation
- `AnyHooksInstalled` short-circuits on first installed tool instead of checking all
- `applyEnvOverrides` extracted as a method on `Config`, shared by `Load` and `LoadWithFile`
- `addAuth` reuses pre-loaded credentials via new `addJWTAuthWithCreds` to avoid double credential loading
- Scanner aggregator delegates to `models.IsLLMCallEvent`/`IsToolCallEvent` instead of local map lookups

### Removed
- Per-tool normalizer files (`normalizer_claude.go`, `normalizer_cursor.go`, `normalizer_gemini.go`, `normalizer_copilot.go`, `normalizer_windsurf.go`) consolidated into single `toolMappings` table
- Duplicate `getMachineID` in `internal/auth/encryption.go` (now uses `device.GetRawHardwareID`)
- Duplicate `sanitizePath` implementations in handler and aggregator (now `models.SanitizePath`)
- Duplicate `syncScanWithJWT` and `patchSessionEnd` in handler (now `api.SendScanWithJWT` / `api.PatchSessionEnd`)
- `hookHTTPClient`, `maxResponseSize` from handler; `maxResponseSize` from API client; `HTTPClient`, `MaxResponseSize` from auth (all consolidated in `httputil`)
- `loadFromEncryptedCache` wrapper that only delegated to `ReadEncryptedCache`
- `calculateFingerprint`, `calculateFilesHash`, `calculateActionCounts` from aggregator and their tests (computed server-side)

## [0.12.0] - 2026-02-27

### Added
- HMAC-SHA256 API key authentication: `hmac_key` config field signs requests so the raw secret never leaves the client; falls back to legacy bcrypt mode when only `secret` is set
- `intentra login --force` flag to re-authenticate without logging out first
- `INTENTRA_API_HMAC_KEY` environment variable for HMAC key configuration
- `Label` metadata on OS keyring entries for better visibility in macOS Keychain and similar tools
- `tableNormalizer` generic type for event normalization, replacing per-tool normalizer structs
- `toolOps` registry for hook install/uninstall/status, replacing per-tool switch statements
- Unified `mergeHookEntries`, `isIntentraEntry`, and `removeIntentraFromHooks` helpers replacing duplicated per-tool hook management functions
- Shared `auth.HTTPClient` (30s timeout) and exported `auth.MaxResponseSize` replacing scattered per-function HTTP clients
- Cache key lookup from OS keyring before falling back to file in `readCacheKey`
- Comprehensive test coverage: `EstimateCost`, `CalculateFingerprint`, `CalculateFilesHash`, `CalculateActionCounts`, `AggregateFilesModified`, `SanitizePath`, `BuildAPIPayload`, `SanitizeMCPServerURL`, `SanitizeMCPServerCmd`, `ParseMCPDoubleUnderscoreName`, `MCPServerURLHash`, `IsMCPEvent`, auth credential tests, archive tests

### Changed
- `GetConfigDir`, `GetDataDir`, `GetEventsFile`, `GetScansDir`, `GetEvidenceDir`, `GetCredentialsFile`, `GetConfigPath` now return `(string, error)` instead of panicking or calling `os.Exit`
- `GetValidCredentials` returns `(*Credentials, error)` with structured errors instead of silent `nil` on failures
- `RefreshCredentials` extracted HTTP call into `doRefreshHTTP`; refresh now runs outside lock with re-check pattern to reduce lock contention
- `createAggregatedScan` decomposed into `initScan`, `aggregateEventMetrics`, `detectFirstString`, `extractSessionEndMetadata`
- `normalizeHookEvent` decomposed into `extractIdentifiers`, `extractToolMetadata`, `extractToolIO`, `extractContentFields`, `extractErrorFields`
- `ProcessEventWithEvent` decomposed into `deriveSessionKey`, `handleStopEvent`, `handleSessionEndEvent`
- API key validation uses `req.URL.Scheme` instead of string prefix matching
- `intentra login` when already logged in now returns success with guidance instead of an error
- `INTENTRA_TOKEN` env var now warns on stderr and sets proper `ExpiresAt` and `RefreshToken` fields
- Compaction percent clamping uses `min(max(...))` builtins
- Manager test uses `strings.Contains` instead of custom `contains` function

### Removed
- Cleartext credential storage: `LoadCredentials`, `SaveCredentials`, `DeleteCredentials` functions
- `MigrateToSecureStorage` and `loadFromCleartextAndMigrate` migration path (users with cleartext credentials must re-login)
- Per-tool normalizer structs (`ClaudeNormalizer`, `CursorNormalizer`, `GeminiNormalizer`, `CopilotNormalizer`, `WindsurfNormalizer`) replaced by `tableNormalizer`
- Per-tool hook management functions (`removeIntentraHooks`, `removeIntentraHooksFromMap`, `removeIntentraHooksFromCopilot`, `removeIntentraHooksFromGemini`, `mergeHooks`, `mergeHookMaps`, `mergeGeminiHooks`) replaced by unified helpers
- Duplicate `maxResponseSize` constants and per-function `http.Client` instances

### Fixed
- `SendScan` now checks `io.ReadAll` error instead of discarding it
- `saveLastScanID` and `clearLastScanID` now handle write/remove errors with debug logging
- `extractCopilotMCP` uses `_` for unused parameter

### Security
- HMAC-SHA256 signing mode prevents raw API key secrets from being transmitted over the wire
- Cleartext credential fallback path removed; credentials are stored only in OS keyring or AES-256-GCM encrypted cache

## [0.11.0] - 2026-02-25

### Added
- Expanded model pricing: Claude 4.5 series (Opus, Sonnet, Haiku), Claude Opus 4, Gemini 3/2.5/2.0/1.5 series, GPT-5.2/o3-pro/o3/o1 models
- Tool-specific pricing multipliers (Windsurf 1.2x; Cursor, Copilot, Claude, Gemini at 1.0x)
- Path sanitization: absolute home directory paths replaced with `~` in scan payloads and aggregated file statistics
- Event loading cap (10,000 events max) to prevent memory exhaustion on large event files
- Malformed event line counting with stderr warning
- `conversationId` extraction in Cursor and Claude normalizers
- Tool extraction from events for cost estimation

### Changed
- Default model updated from `claude-3-5-sonnet` to `claude-sonnet-4.5`
- Default fallback cost updated from $0.003 to $0.005 per 1K tokens
- Scan IDs now use 12-byte hashes (24 hex chars) instead of 8-byte (16 hex chars) for lower collision probability
- `EstimateCost` uses sorted prefix matching (longest prefix first) for correct model identification
- Credential migration from cleartext to secure storage now runs synchronously to prevent goroutine leak in short-lived CLI process
- Unicode-safe `capitalizeFirst` using `unicode.ToUpper`
- Debug HTTP logs now include RFC3339 timestamps
- Removed unused `body` parameter from `addAuth`

### Fixed
- Hook template generators (`GenerateCursorHooksJSON`, `GenerateCopilotHooksJSON`, `GenerateWindsurfHooksJSON`) now return proper errors instead of swallowing `json.MarshalIndent` failures
- Lock file write errors now propagated instead of silently ignored
- `isProcessRunning` correctly handles `ErrPermission` (process exists but owned by another user)
- `GetConfigDir` now exits with error message when home directory cannot be determined (was silently using empty path)
- Warning logged when credential lock acquisition fails during token refresh

### Security
- Config file permission check warns if group/other-readable (`0077` mask)
- Atomic config file writes via temp file + rename with `0600` permissions
- Auto-expiry of stale lock files older than 60 seconds
- Auth HTTP client uses explicit 30-second timeout instead of unbounded default
- Lock file release failures logged to stderr

## [0.10.0] - 2026-02-19

### Added
- `BuildAPIPayload` method on Scan model, consolidating duplicated payload construction from API client and hook handler
- HTTPS-only enforcement for API key authentication (config validation and runtime check)
- URL validation in `openBrowser` (rejects non-HTTPS URLs)
- `url.PathEscape` for all user-supplied path segments in API URLs
- `io.LimitReader` (10 MB cap) on all HTTP response body reads to prevent memory exhaustion
- Context timeouts (5 seconds) on all external command executions (`ioreg`, `reg query`, `sw_vers`)
- `sync.Once` for thread-safe keyring initialization
- Panic recovery in background keystore migration goroutine
- Larger `bufio.Scanner` buffers (10 MB) for reading large JSONL event files
- Stale buffer cleanup throttling (at most once per 30 minutes)
- Proper JSON escaping for string `tool_output` values via `json.Marshal`

### Changed
- Replaced MD5 with SHA-256 for fingerprint and files hash calculations
- Moved `defaultAPIEndpoint` to `config.DefaultAPIEndpoint` (single source of truth)
- Moved User-Agent constant to `api.UserAgent`
- Event normalizer maps promoted from function-scoped to package-level variables
- Config generation in `saveAPIConfig` uses `yaml.Marshal` instead of string interpolation
- Reused shared `http.Client` instances instead of creating new ones per request
- Windows `openBrowser` uses `rundll32` instead of `cmd /c start`
- Replaced `interface{}` with `any` throughout
- Cost estimation in hook handler now delegates to `scanner.EstimateCost`

### Removed
- `EstimateTokens` function and tests (unused)
- `VerifyDeviceID` function and tests (unused)
- `GetValidCredentialsSecure` alias (was identical to `GetValidCredentials`)
- `Health` method on API client (unused)
- `ProcessEvent`, `RunHookHandler`, `RunHookHandlerWithTool` wrappers (unused)
- `CreateScanFromEvent` function (unused)
- `TryWithCredentialLock` function (unused)
- `ScanContent` struct and `Content` field on Scan (unused)
- `ScanStatusRejected` constant (unused)
- `Scan.Duration()` and `Scan.AddEvent()` methods (unused)
- `RefundLikelihood`, `RefundAmount`, `Summary`, `AcceptedLines`, `Timestamp` fields from Scan (unused)
- `IntentraVersion` from `DeviceMetadata` and `Version` variable (build version tracked elsewhere)
- Legacy compatibility methods: `Config.APIKey()`, `Config.Model()`, `GetCursorHooksDir()`, `GenerateHooksJSON()`
- Duplicate darwin case in `getWindsurfHooksDir`
- Inline model pricing map in hook handler (replaced by `scanner.EstimateCost`)

### Security
- All HTTP response reads capped at 10 MB to prevent denial-of-service via large payloads
- API key credentials refused over non-HTTPS connections
- Browser open command validates HTTPS scheme before execution
- SHA-256 replaces MD5 for internal hashing
- External commands run with 5-second timeouts to prevent indefinite hangs

## [0.9.1] - 2026-02-18

### Changed
- Added logo image to README header
- Renamed top-level heading from "Intentra CLI" to "Overview"

## [0.9.0] - 2026-02-17

### Added
- Git metadata collection: repo name, repo URL hash, and branch name attached to each scan
- File modification tracking: per-file edit statistics (lines added/removed, edit count, new file detection) aggregated from events
- Session end tracking with reason and duration fields on Scan model
- PATCH `/scans/{id}/session` API call to update session end metadata on previously synced scans
- Last scan ID persistence per session for linking session end events to their scan
- New normalized event types: `subagent_start`, `tool_use_failure`
- New Cursor hook event types: `beforeSubmitPrompt`, `postToolUseFailure`, `subagentStart`, `subagentStop`, `beforeMCPExecution`, `afterAgentResponse`, `afterAgentThought`, `beforeReadFile`
- `AggregateFilesModified` function in scanner aggregator for building per-file edit statistics
- `SessionEndReason`, `SessionDurationMs`, `RepoName`, `RepoURLHash`, `BranchName`, `FilesModified`, `AcceptedLines` fields on Scan model

### Changed
- `IsStopEvent` now uses per-tool terminal event mapping to prevent duplicate scans (Windsurf uses `afterResponse`, Copilot/Gemini use `sessionEnd`, others use `stop`)
- `IsSessionEndEvent` introduced for tools where session end is a separate PATCH rather than a scan trigger
- Stale buffer cleanup now also cleans up `intentra_lastscan_*.txt` temp files
- Scan submission payload now includes git metadata, session end, and file modification fields

## [0.8.2] - 2026-02-13

### Fixed
- Encrypted cache key file now removed before rewrite to prevent permission denied error on subsequent logins

## [0.8.1] - 2026-02-07

### Fixed
- Error field now correctly populated on Event when hook payload contains error object or string
- Aligned map literal formatting in MCP tool-to-server mapping

## [0.8.0] - 2026-02-06

### Added
- Context compaction event tracking with `preCompact` event type support for Cursor
- Compaction metadata extraction: trigger type, context usage percent, context/window token counts, message counts, and first-compaction flag
- Seven new fields on Event model: `ContextUsagePercent`, `ContextTokens`, `ContextWindowSize`, `MessageCount`, `MessagesToCompact`, `IsFirstCompaction`, `CompactionTrigger`
- Compaction metadata included in scan submission payload to API
- Rate limiting for `pre_compact` events (max 10 per scan) to prevent payload bloat

## [0.7.0] - 2026-02-06

### Added
- MCP (Model Context Protocol) tool attribution: per-server and per-tool usage tracking across all supported AI coding tools
- `MCPToolCall` model for aggregated MCP tool usage within scans
- `extractMCPMetadata` with per-tool extraction for Cursor, Windsurf, Claude Code, Gemini CLI, and GitHub Copilot
- `aggregateMCPToolUsage` for proportional cost attribution based on MCP call duration vs total scan duration
- `inferMCPServerName` with known tool-to-server mapping for common MCP servers (cursor-browser, chrome-devtools, sentry, posthog)
- `IsMCPEvent()` method on Event for MCP event identification
- MCP data sanitization utilities (`SanitizeMCPServerURL`, `SanitizeMCPServerCmd`) to prevent leaking API keys and local paths
- `ParseMCPDoubleUnderscoreName` for Claude Code and Gemini CLI `mcp__server__tool` format
- `MCPServerURLHash` for server deduplication across different connection methods
- `mcp_tool_usage` field included in scan submission payload to API
- MCP action count tracking in scan aggregator (`action_counts.mcp`)

### Fixed
- Corrected indentation in credential file lock write (filelock.go)

## [0.6.0] - 2026-02-05

### Added
- Secure credential storage using OS-native keyring (macOS Keychain, Windows Credential Manager, Linux Secret Service/KeyCtl)
- Encrypted file cache (`~/.intentra/credentials.enc`) for hook handlers using AES-256-GCM encryption
- Unified file locking (`~/.intentra/credentials.lock`) for cross-application credential coordination
- Check-before-write pattern in token refresh to prevent race conditions between CLI and IDE extensions
- `github.com/99designs/keyring` dependency for cross-platform keyring support
- `golang.org/x/crypto` dependency for HKDF key derivation

### Security
- Credentials no longer stored in cleartext JSON; now encrypted or in OS keyring
- Machine-derived fallback encryption key using HKDF-SHA256 for headless environments
- PID-based stale lock detection to clean up locks from crashed processes

### Changed
- `intentra login` now stores credentials in OS keyring with encrypted cache fallback
- `intentra logout` removes credentials from both keyring and encrypted cache
- `intentra status` loads credentials from secure storage hierarchy
- Token refresh now uses file locking to coordinate between CLI and IDE hooks

## [0.5.0] - 2026-02-05

### Added
- Claude-to-Cursor session merging: Claude events with matching Cursor session buffers are now attributed to Cursor
- Extended Gemini CLI event types: SessionStart, SessionEnd, BeforeAgent, AfterAgent, BeforeToolSelection, PreCompress, Notification

### Changed
- Updated CLI help text to list all supported tools (Cursor, Claude Code, Gemini CLI, GitHub Copilot, Windsurf)

### Fixed
- Cursor hooks status check now correctly detects installed hooks (was requiring `enabled: true` flag)

## [0.4.0] - 2026-02-02

### Changed
- **Breaking**: Removed HMAC signature authentication mode
- **Breaking**: Removed mTLS certificate authentication mode
- Simplified `AuthConfig` struct to only support `api_key` mode (or JWT via `intentra login`)
- Default auth mode is now empty string (uses JWT from `intentra login`)
- Updated README with Windows PowerShell installation instructions
- Updated example configs to reflect new `api_key` auth format
- Simplified API client by removing HMAC signing and mTLS configuration code

### Removed
- `HMACConfig` struct from config package
- `MTLSConfig` struct from config package
- `examples/config-mtls.yaml` example file
- HMAC signature generation (`signRequest`, `setAuthHeaders`) from API client
- mTLS certificate loading (`configureMTLS`) from API client

## [0.3.5] - 2026-02-01

### Added
- Test coverage for `internal/debug` package (Log, LogHTTP, Warn functions)
- Test coverage for `internal/device` package (GetDeviceID, VerifyDeviceID, GetMetadata)
- Tests for new Event fields (GenerationID, Error)
- Tests for EstimateTokens function with table-driven test cases
- Tests for Scan model with cross-scan detection fields (fingerprint, files_hash, action_counts)

### Changed
- Handler tests updated to match new resilient sync behavior (warnings instead of errors)

## [0.3.0] - 2026-02-01

### Added
- `--debug` (`-d`) global flag for debug output (HTTP request logging, local scan saves)
- `debug: true` config option for persistent debug mode (works for hooks called by IDEs)
- New `internal/debug` package with `Log`, `LogHTTP`, and `Warn` functions
- HTTP request logging showing method, URL, and status code in debug mode
- Local scan persistence to `~/.intentra/scans/` when debug mode enabled
- Config file auto-generation on first run with defaults
- `SaveConfig` function in config package for persisting configuration changes
- `ConfigExists` and `GetConfigPath` helper functions
- Cross-scan pattern detection support (Pro/Enterprise feature):
  - Scan fingerprint calculation for duplicate task detection
  - Files hash aggregation for cross-scan retry detection
  - Action counts (edits, reads, shell, failed) for session analysis
- New scan payload fields: `fingerprint`, `files_hash`, `action_counts`
- `GenerationID` field on Event and Scan models for turn/execution tracking
- `Model` field on Scan model for model identification
- `Error` field on Event model for error tracking
- `scripts/pre-commit` hook for development

### Changed
- **Breaking**: Config file location changed from `~/.config/intentra/config.yaml` to `~/.intentra/config.yaml`
- **Breaking**: Scans location changed from `~/.local/share/intentra/scans/` to `~/.intentra/scans/`
- **Breaking**: Gemini CLI hooks now use matcher-based format with named hooks and timeouts
- Using `-d` flag now persists `debug: true` to config file automatically
- Hooks now check for valid JWT credentials first, then fall back to config-based auth
- Logged-in users (`intentra login`) now automatically sync to api.intentra.sh
- Cursor hooks directory changed from `~/.cursor/hooks/` to `~/.cursor/`
- Buffer feature disabled by default (`buffer.enabled: false`)
- Uninstall commands now preserve non-intentra hooks instead of deleting entire hooks.json
- Claude Code uninstall now preserves non-intentra hooks
- Gemini CLI uninstall now preserves non-intentra hooks using matcher-aware removal
- Copilot uninstall now preserves non-intentra hooks
- Windsurf uninstall now preserves non-intentra hooks
- Scan model extended with cross-scan detection metadata
- Aggregator now calculates fingerprint hashes during scan creation
- Hook handler now uses `generation_id` from raw events (maps `execution_id`, `turn_id`)
- Improved JSON unmarshal error handling in hook installation

### Fixed
- `intentra login` now enables data syncing (hooks check JWT credentials before server.enabled)
- Scans are synced to api.intentra.sh when logged in, even without server.enabled in config
- Sync command now preserves local files when debug mode is enabled
- Model extracted from events and set on scan correctly (was always defaulting)

## [0.2.0] - 2026-01-26

### Added
- `intentra install [tool]` as top-level command (replaces `intentra hooks install`)
- `intentra uninstall [tool]` as top-level command (replaces `intentra hooks uninstall`)
- GitHub Copilot hook support (normalizer + hook installation)
- Windsurf Cascade hook support (normalizer + hook installation)
- `NormalizedType` field on Event struct for unified event classification across tools
- `RawEvents` field on Scan struct for forwarding raw hook data to backend
- `make intentra` command to build and install CLI to ~/bin/intentra
- Automatic token refresh using refresh tokens (sessions no longer expire after 24 hours)
- Server-side token/cost estimation for Claude Code and Gemini CLI hooks (tools that don't provide usage data)
- Scans now include conversation_id and session_id for traceability across tools
- Gemini CLI added to tools dropdown in frontend
- **Event aggregation**: Hook events are now buffered and sent as a single scan on "stop" event, enabling proper violation detection (retry loops, tool misuse, etc.)

### Changed
- **Normalizer architecture refactored**: Simplified interface with auto-registration pattern; normalizers only convert event types, raw data forwarded to backend
- `intentra login` now fails with error if already logged in (must logout first)
- Increased device registration timeout from 10s to 60s to handle Lambda cold starts
- All commands now suppress usage/help output on errors for cleaner CLI experience
- Scans page now filters out $0 cost scans by default (can be toggled)
- Scan detail page shows conversation_id (Cursor) or session_id (Claude/Gemini)
- Hook events buffered in temp directory (auto-cleared after 30 minutes or on successful send) - no permanent local storage for server-sync mode
- Scan API payload simplified to single-scan format with flat structure (was batch with nested `scans` array)
- Hook handler no longer requires server validation to start (silent no-op when server disabled)

### Removed
- `intentra hooks install` and `intentra hooks uninstall` commands (use `intentra install` and `intentra uninstall`)
- SQLite-based offline buffer (`internal/buffer`) - server-sync mode now uses temp file buffering only
- Violation model (`pkg/models/violation.go`) - violation detection moved to backend
- `NormalizedEvent` struct - raw events forwarded directly to backend for processing
- `Retries` field from Scan struct - retry detection moved to backend
- Hardcoded `HookType` constants - native event types preserved, normalized separately

### Fixed
- Hooks now send scans using JWT auth (was incorrectly using API key headers)
- Scan submission now accepts HTTP 201 Created response
- Scan submission payload now matches backend schema (was sending nested `scans` array)
- Hook data now properly normalized from Cursor/Claude Code/Gemini CLI (maps field names like `hook_event_name`→`hook_type`, `tool_response`→`tool_output`, `duration`→`duration_ms`, extracts `command` from `tool_input`)
- Raw hook events now forwarded to backend for violation detection (all three tools provide rich data: `tool_name`, `tool_input`, `tool_output`/`tool_response`, `duration`, `session_id`/`conversation_id`)
- Backend violation detector now recognizes `hook_type` field from CLI events
- Single-event retry detection for obvious patterns ("retrying", "connection refused", etc.)
- Machines Lambda now has access to orgs table for plan resolution
- Version now correctly reported in device metadata (was always showing "dev")
- Added ldflags for `internal/device.Version` in Makefile and GoReleaser config
- Status message now shows "Not logged in" instead of "Not authenticated"
- Help text now correctly references `intentra login` instead of `intentra auth login`
- Backend token endpoint now returns OAuth2-compliant error format for device flow polling

## [0.1.3] - 2026-01-25

### Added
- Auto-register device on login via `POST /machines`
- Handle device limit errors with upgrade prompt
- Handle admin-revoked device errors with support message

### Changed
- Simplified fallback device ID to `hostname:username` (removed home directory component)

## [0.1.2] - 2026-01-25

### Added
- `login`, `logout`, `status` commands for CLI authentication (OAuth device flow)
- `scan today` command to filter scans by current date
- API GET methods for server-mode scan queries (`GetScans`, `GetScan`)
- `--keep-local` flag for sync command to preserve local files
- Source-aware scan commands (API vs local based on server.enabled)
- Token management in `internal/auth/token.go`

### Changed
- `scan list` now queries API when server mode enabled
- `scan show` now queries API when server mode enabled
- `sync now` deletes local files after successful sync (unless `--keep-local`)
- Simplified auth commands to top-level (no `auth` subcommand)
- Updated README to emphasize local-first mode with optional intentra.sh server sync

### Fixed
- Scans no longer persist locally after successful server sync
- Source of truth now correctly follows server.enabled configuration

## [0.1.1] - 2026-01-24

### Changed
- Simplified README.md to match CLI implementation

### Removed
- SECURITY.md

### Fixed
- Hook command now accepts --event flag for proper event categorization
- Fixed redundant code and error handling in Cursor hook installation
- Fixed incomplete path traversal validation in scan loading
- Updated model pricing to use prefix matching for accurate cost estimates

## [0.1.0] - 2026-01-18

### Added
- Initial release
- Hook management for Cursor, Claude Code, and Gemini CLI
- Event normalization across AI tools
- Scan aggregation
- Local storage with optional server sync
- HMAC authentication for server sync

[0.15.0]: https://github.com/atbabers/intentra-cli/compare/v0.14.1...v0.15.0
[0.14.1]: https://github.com/atbabers/intentra-cli/compare/v0.14.0...v0.14.1
[0.14.0]: https://github.com/atbabers/intentra-cli/compare/v0.13.0...v0.14.0
[0.13.0]: https://github.com/atbabers/intentra-cli/compare/v0.12.0...v0.13.0
[0.12.0]: https://github.com/atbabers/intentra-cli/compare/v0.11.0...v0.12.0
[0.11.0]: https://github.com/atbabers/intentra-cli/compare/v0.10.0...v0.11.0
[0.10.0]: https://github.com/atbabers/intentra-cli/compare/v0.9.1...v0.10.0
[0.9.1]: https://github.com/atbabers/intentra-cli/compare/v0.9.0...v0.9.1
[0.9.0]: https://github.com/atbabers/intentra-cli/compare/v0.8.2...v0.9.0
[0.8.2]: https://github.com/atbabers/intentra-cli/compare/v0.8.1...v0.8.2
[0.8.1]: https://github.com/atbabers/intentra-cli/compare/v0.8.0...v0.8.1
[0.8.0]: https://github.com/atbabers/intentra-cli/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/atbabers/intentra-cli/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/atbabers/intentra-cli/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/atbabers/intentra-cli/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/atbabers/intentra-cli/compare/v0.3.5...v0.4.0
[0.3.5]: https://github.com/atbabers/intentra-cli/compare/v0.3.0...v0.3.5
[0.3.0]: https://github.com/atbabers/intentra-cli/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/atbabers/intentra-cli/compare/v0.1.3...v0.2.0
[0.1.3]: https://github.com/atbabers/intentra-cli/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/atbabers/intentra-cli/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/atbabers/intentra-cli/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/atbabers/intentra-cli/releases/tag/v0.1.0
