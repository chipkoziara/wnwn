import { app } from 'electron';
import { spawn, ChildProcess } from 'child_process';
import * as net from 'net';
import * as path from 'path';

let serverProcess: ChildProcess | null = null;

/** Finds a free TCP port on 127.0.0.1. */
function findFreePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const srv = net.createServer();
    srv.unref();
    srv.listen(0, '127.0.0.1', () => {
      const addr = srv.address();
      if (!addr || typeof addr === 'string') {
        reject(new Error('Could not determine free port'));
        return;
      }
      srv.close(() => resolve(addr.port));
    });
    srv.on('error', reject);
  });
}

/** Returns the path to the bundled wnwn-server binary. */
function serverBinaryPath(): string {
  const binaryName = process.platform === 'win32' ? 'wnwn-server.exe' : 'wnwn-server';

  if (app.isPackaged) {
    // In production the binary is in the app's Resources directory.
    return path.join(process.resourcesPath, binaryName);
  }

  // In development, use the built binary from the repo root.
  return path.join(app.getAppPath(), '..', binaryName);
}

/**
 * Starts the wnwn-server binary and waits for its JSON startup handshake.
 * Returns the base URL (e.g. "http://127.0.0.1:9274").
 *
 * @param onCrash  Called if the server exits unexpectedly after startup.
 */
export async function startServer(
  onCrash: (code: number | null) => void
): Promise<string> {
  const port = await findFreePort();
  const addr = `127.0.0.1:${port}`;
  const binary = serverBinaryPath();

  serverProcess = spawn(binary, ['--addr', addr], {
    env: { ...process.env },
    stdio: ['ignore', 'pipe', 'pipe'],
  });

  // Forward server stderr to the Electron process stderr for debugging.
  serverProcess.stderr?.on('data', (chunk: Buffer) => {
    process.stderr.write(`[wnwn-server] ${chunk}`);
  });

  // Wait for the startup handshake printed on stdout.
  const baseUrl = await waitForHandshake(serverProcess, addr);

  let started = true;
  serverProcess.on('exit', (code) => {
    if (!started) return; // killed intentionally by stopServer()
    serverProcess = null;
    onCrash(code);
  });

  // Suppress the exit handler after intentional stop.
  serverProcess.once('close', () => {
    started = false;
  });

  return baseUrl;
}

/** Sends SIGTERM to the server process and clears the reference. */
export function stopServer(): void {
  if (serverProcess) {
    serverProcess.kill('SIGTERM');
    serverProcess = null;
  }
}

/**
 * Waits for the server to print {"ready":true,"addr":"..."} on stdout.
 * Times out after 10 seconds.
 */
function waitForHandshake(proc: ChildProcess, expectedAddr: string): Promise<string> {
  return new Promise((resolve, reject) => {
    const timeout = setTimeout(() => {
      reject(new Error(`wnwn-server did not start within 10 seconds (expected addr: ${expectedAddr})`));
    }, 10_000);

    let buffer = '';

    proc.stdout?.on('data', (chunk: Buffer) => {
      buffer += chunk.toString();
      // The handshake is a single JSON line.
      const lines = buffer.split('\n');
      for (const line of lines) {
        const trimmed = line.trim();
        if (!trimmed) continue;
        try {
          const msg = JSON.parse(trimmed) as { ready?: boolean; addr?: string };
          if (msg.ready && msg.addr) {
            clearTimeout(timeout);
            resolve(`http://${msg.addr}`);
            return;
          }
        } catch {
          // Not JSON yet — keep buffering.
        }
      }
      // Keep only the last partial line.
      buffer = lines[lines.length - 1];
    });

    proc.on('error', (err) => {
      clearTimeout(timeout);
      reject(new Error(`Failed to spawn wnwn-server: ${err.message}`));
    });

    proc.on('exit', (code) => {
      clearTimeout(timeout);
      reject(new Error(`wnwn-server exited before becoming ready (code ${code})`));
    });
  });
}
