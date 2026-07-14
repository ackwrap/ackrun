# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a monorepo containing multiple proxy subscription management and proxy tool projects. The main components are:

- **miaomiaowu**: Go-based subscription management server with web UI (primary application)
- **backend**: Separate Go backend service using Gin framework
- **frontend**: Vue 3 + TypeScript web application (Vite)
- **mihomo-Alpha**: Fork of Clash Meta proxy tool
- **sing-box**: Alternative proxy implementation

## Build and Run Commands

### miaomiaowu (Primary Application)

```bash
# Build the server
cd miaomiaowu
go build -o ../bin/miaomiaowu ./cmd/server

# Run the server (from project root or miaomiaowu directory)
./bin/miaomiaowu
# Or from miaomiaowu directory:
go run ./cmd/server

# The server expects these directories:
# - subscribes/ (subscription files)
# - rule_templates/ (Clash rule templates)
# - data/ (SQLite database: traffic.db)
```

### backend

```bash
cd backend
go run ./cmd/server
# Or build:
go build -o ../bin/backend ./cmd/server
```

### frontend

```bash
cd frontend
npm install         # Install dependencies
npm run dev         # Development server
npm run build       # Production build
npm run preview     # Preview production build
```

### mihomo-Alpha (Clash Proxy)

```bash
cd mihomo-Alpha
make linux-amd64-v3      # Build for Linux AMD64
make darwin-arm64        # Build for macOS ARM64
make windows-amd64-v3    # Build for Windows AMD64
make all                 # Build common platforms

# Built binaries go to: mihomo-Alpha/bin/
```

### sing-box

```bash
cd sing-box
make                # Build binary
# Check Makefile for specific build targets
```

## Architecture

### miaomiaowu Server Architecture

**Entry Point**: `miaomiaowu/cmd/server/main.go`

Key components initialized on startup:
1. Logger system (`internal/logger`)
2. Traffic database (SQLite: `data/traffic.db`)
3. Authentication manager with token-based sessions
4. Subscription file management
5. Rule template system with DNS patch application
6. Proxy groups configuration (fetched from remote or fallback to empty)
7. Notification module (Telegram bot integration)
8. Background schedulers:
   - Proxy provider cache sync
   - Traffic summary collection
   - Notification scheduler (daily traffic, expiry alerts)
   - Log cleanup (daily at 3 AM, removes 7-day-old logs)

**HTTP Server**: Standard library `net/http` with manual routing via `http.ServeMux`

**Key Subsystems**:
- `internal/storage`: SQLite repository layer (using modernc.org/sqlite)
- `internal/handler`: HTTP handlers for all endpoints
- `internal/auth`: User authentication, 2FA, session management
- `internal/substore`: Subscription parsing and generation
- `internal/proxygroups`: Proxy group configuration management
- `internal/speedtest`: Speed testing infrastructure
- `internal/notify`: Telegram notification system
- `internal/scriptengine`: JavaScript runtime for script overrides (uses dop251/goja)

**Frontend Integration**: The TypeScript frontend (`miaomiaowu/miaomiaowu/src/`) builds Clash configurations via `lib/sublink/clash-builder.ts`, handling:
- Proxy conversion and ordering
- Rule provider generation
- Proxy group construction
- YAML serialization

### backend Architecture

Uses **Gin framework** for HTTP routing. Appears to be a separate API service.

### Frontend Architecture

**Tech Stack**: Vue 3 + TypeScript + Vite + Vue Router + Tailwind CSS 4 + DaisyUI

**Structure**:
- `src/components/`: Reusable UI components
- `src/pages/`: Page-level components
- `src/services/`: API client layer
- `src/composables/`: Vue composables
- `src/utils/`: Utility functions

## Key Design Patterns

### Subscription System

Short links use a dual-code system:
- Format: `/{subscriptionCode}{userCode}` (default: 3+3 chars, customizable to min 1+1)
- Temporary subscriptions: `/t/{id}` (10 chars: "t/" + 8 hex chars)
- Brute force protection with IP-based rate limiting and blocking
- Subscription rate limiting (configurable max requests per window)

### Authentication Flow

1. Login with username/password → optional 2FA challenge → JWT-like token
2. Token stored in `auth.TokenStore` (in-memory, 24-hour expiry)
3. Sessions persisted to SQLite for recovery across restarts
4. Middleware: `auth.RequireToken()` and `auth.RequireAdmin()`

### Configuration Generation

The system generates Clash/Clash.Meta configurations dynamically:
1. Fetch user's subscription sources (external URLs or uploaded files)
2. Parse proxy nodes from various formats
3. Apply user-selected rule categories and custom rules
4. Build proxy groups (Node Select, Auto Select, category-specific groups)
5. Generate rule-providers with remote rule sets
6. Serialize to YAML with specific field ordering

## Database Schema

SQLite database (`data/traffic.db`) managed by `internal/storage/repository.go`:
- `users`: User accounts, passwords (bcrypt), 2FA secrets, traffic limits
- `subscriptions`: User subscription configurations
- `subscribe_files`: Uploaded or synced subscription sources
- `sessions`: Persistent login sessions
- `traffic_records`: Historical traffic usage
- `route_rules`: Custom routing rules
- `speed_testers`: Speed test endpoint registration
- `custom_rules`: User-defined routing rules
- `override_scripts`: JavaScript override scripts

## Testing

The codebase has minimal test coverage. Test files found:
- `backend/internal/store/*_test.go`
- `mihomo-Alpha/test/*_test.go`
- Some unit tests in sing-box

To run tests:
```bash
# For Go projects
cd miaomiaowu  # or backend, mihomo-Alpha, sing-box
go test ./...

# Run specific package tests
go test ./internal/storage
go test ./internal/auth

# With verbose output
go test -v ./...
```

## Development Notes

### Environment Variables

miaomiaowu server reads:
- `PORT`: HTTP server listen port (default: from `getAddr()` function)
- Check `miaomiaowu/cmd/server/main.go` for other environment variables

### CORS Configuration

CORS origins determined by `getAllowedOrigins()` function in main.go. The server includes a CORS middleware wrapper.

### Silent Mode

The system implements a "silent mode" via `handler.SilentModeManager` middleware that can suppress certain notifications or behaviors based on system configuration.

### Proxy Provider Cache

Proxy providers are cached and synced periodically. The cache can be manually refreshed via API endpoints. Initial cache population happens on startup in a goroutine.

### Rule Templates

Rule templates in `rule_templates/` directory:
- `fake_ip__v3.yaml`
- `redirhost__v3.yaml`
- Automatically patched for known DNS configuration issues via `internal/patches`

## Dependencies

### Go Modules

- **miaomiaowu**: Go 1.24.0, modernc.org/sqlite, gorilla/websocket, gopkg.in/yaml.v3, dop251/goja (JS engine)
- **backend**: Go 1.26.3, gin-gonic/gin, gorilla/websocket, modernc.org/sqlite
- **mihomo-Alpha**: Extensive networking and proxy protocol libraries
- **sing-box**: Similar proxy protocol dependencies

### Frontend

- Vue 3.5, Vue Router 5
- TypeScript 6
- Vite 7 (build tool)
- Tailwind CSS 4 + DaisyUI 5 (styling)
- lucide-vue-next (icons)

## Common Tasks

### Adding a New API Endpoint (miaomiaowu)

1. Create handler in `internal/handler/your_handler.go`
2. Implement `ServeHTTP(w http.ResponseWriter, r *http.Request)` method
3. Add route in `cmd/server/main.go` (around line 150-250)
4. Apply auth middleware if needed: `auth.RequireToken()` or `auth.RequireAdmin()`

### Modifying Subscription Generation

Edit `miaomiaowu/miaomiaowu/src/lib/sublink/clash-builder.ts`:
- `convertProxies()`: Proxy node conversion
- `buildProxyGroups()`: Proxy group structure
- `buildRuleProviders()`: Rule provider configuration
- `buildRules()`: Rule ordering and formatting

### Database Migrations

No formal migration system. Schema changes require manual SQL or adding to `internal/storage/repository.go` initialization.
