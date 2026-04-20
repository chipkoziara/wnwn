#!/usr/bin/env node
/**
 * build-go.js
 *
 * Cross-compiles wnwn-server for all distribution targets and places the
 * binaries in electron-client/resources/ where electron-builder picks them up.
 *
 * Targets:
 *   darwin/arm64  →  resources/wnwn-server-darwin-arm64
 *   darwin/amd64  →  resources/wnwn-server-darwin-x64
 *   linux/amd64   →  resources/wnwn-server-linux-x64
 *
 * Usage:
 *   node scripts/build-go.js              # all targets
 *   node scripts/build-go.js --mac        # darwin targets only
 *   node scripts/build-go.js --linux      # linux targets only
 *   node scripts/build-go.js --current    # current OS/arch only (fast dev build)
 */

const { execFileSync } = require('child_process');
const fs = require('fs');
const path = require('path');
const os = require('os');

// Resolve paths relative to the repo root (one level up from electron-client/).
const repoRoot = path.resolve(__dirname, '..', '..');
const resourcesDir = path.resolve(__dirname, '..', 'resources');
const serverPkg = './cmd/wnwn-server/';

fs.mkdirSync(resourcesDir, { recursive: true });

const args = process.argv.slice(2);
const onlyMac     = args.includes('--mac');
const onlyLinux   = args.includes('--linux');
const onlyCurrent = args.includes('--current');

/** All distribution targets */
const allTargets = [
  { goos: 'darwin', goarch: 'arm64', out: 'wnwn-server-darwin-arm64' },
  { goos: 'darwin', goarch: 'amd64', out: 'wnwn-server-darwin-x64'  },
  { goos: 'linux',  goarch: 'amd64', out: 'wnwn-server-linux-x64'   },
];

function currentTarget() {
  const goos   = os.platform() === 'darwin' ? 'darwin' : 'linux';
  const goarch = os.arch() === 'arm64' ? 'arm64' : 'amd64';
  const suffix = goos === 'darwin'
    ? `darwin-${goarch}`
    : `linux-${goarch === 'amd64' ? 'x64' : goarch}`;
  return [{ goos, goarch, out: `wnwn-server-${suffix}` }];
}

function selectTargets() {
  if (onlyCurrent) return currentTarget();
  if (onlyMac)    return allTargets.filter(t => t.goos === 'darwin');
  if (onlyLinux)  return allTargets.filter(t => t.goos === 'linux');
  return allTargets;
}

const targets = selectTargets();

console.log(`Building wnwn-server for ${targets.length} target(s)…\n`);

let failed = 0;

for (const { goos, goarch, out } of targets) {
  const outPath = path.join(resourcesDir, out);
  const label = `${goos}/${goarch} → resources/${out}`;
  process.stdout.write(`  ${label} … `);

  try {
    execFileSync('go', ['build', '-trimpath', '-ldflags=-s -w', '-o', outPath, serverPkg], {
      cwd: repoRoot,
      env: { ...process.env, GOOS: goos, GOARCH: goarch, CGO_ENABLED: '0' },
      stdio: ['ignore', 'pipe', 'pipe'],
    });

    // Make executable
    fs.chmodSync(outPath, 0o755);

    const size = (fs.statSync(outPath).size / 1024 / 1024).toFixed(1);
    console.log(`done (${size} MB)`);
  } catch (err) {
    console.log('FAILED');
    console.error(`    ${err.stderr?.toString().trim() ?? err.message}`);
    failed++;
  }
}

console.log('');

if (failed > 0) {
  console.error(`${failed} target(s) failed.`);
  process.exit(1);
} else {
  console.log(`All binaries written to: ${resourcesDir}`);
}
