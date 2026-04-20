# Building & Packaging wnwn Electron Client

## Prerequisites

- Go 1.25+ (`mise` recommended: `eval "$(mise activate bash)"`)
- Node.js 18+ with pnpm (`pnpm install` from this directory)
- pnpm (`npm install -g pnpm` or via mise)

## Dev workflow

```bash
# 1. Install JS deps (first time / after package.json changes)
pnpm install
cd src/renderer && pnpm install && cd ../..

# 2. Build and run Go server (needed for API calls)
cd .. && go build -o wnwn-server ./cmd/wnwn-server/ && cd electron-client

# 3. Start dev mode (two terminals)
pnpm run dev:renderer   # Terminal 1 — Vite dev server on :5173
pnpm start              # Terminal 2 — Electron (loads :5173, auto-builds main TS)
```

## Distribution builds

### Step 1 — Compile Go server binaries for all targets

```bash
node scripts/build-go.js
```

This cross-compiles `wnwn-server` for all platforms and writes to `resources/`:

| File | Platform |
|------|----------|
| `resources/wnwn-server-darwin-arm64` | macOS Apple Silicon |
| `resources/wnwn-server-darwin-x64`   | macOS Intel |
| `resources/wnwn-server-linux-x64`    | Linux x64 |

Options:
```bash
node scripts/build-go.js --mac      # darwin targets only
node scripts/build-go.js --linux    # linux only
node scripts/build-go.js --current  # current OS/arch only (fastest)
```

### Step 2 — Build Electron app

```bash
pnpm run build          # compile renderer (Vite) + main process (tsc)
```

### Step 3 — Package

```bash
# Linux AppImage (x64) — can be built on Linux
pnpm run dist:linux

# macOS DMG (arm64 + x64) — must be built on macOS
pnpm run dist:mac

# Both (cross-platform packaging; macOS DMG still requires macOS host)
pnpm run dist:all
```

Or let `predist` handle steps 2+3 automatically:
```bash
pnpm run dist:linux   # runs build then electron-builder --linux
```

Output lands in `release/`:
- `release/wnwn-0.1.0.AppImage`  (Linux)
- `release/wnwn-0.1.0-arm64.dmg` (macOS Apple Silicon)
- `release/wnwn-0.1.0.dmg`       (macOS Intel)

## macOS notes

- DMG builds **must** run on a macOS host (or macOS CI runner).
- For public distribution, you need an Apple Developer ID certificate
  and notarization. Add to `package.json` `build.mac`:
  ```json
  "identity": "Developer ID Application: Your Name (TEAMID)",
  "notarize": true
  ```
- `notarize` requires `APPLE_ID`, `APPLE_APP_SPECIFIC_PASSWORD`, and
  `APPLE_TEAM_ID` env vars. See electron-builder docs for the full flow.

## App icon

A custom icon should be placed at:
- `build/icon.icns`  — macOS
- `build/icon.ico`   — Windows (future)
- `build/icon.png`   — Linux (512×512 px recommended)

Without an icon, electron-builder uses the default Electron icon.

## Binary sizes

The Go server binary is ~10 MB per platform (stripped with `-s -w`).
The Electron app itself is ~90 MB (Chromium runtime).
Total AppImage / DMG: ~108 MB.
