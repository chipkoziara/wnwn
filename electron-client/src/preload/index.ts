import { contextBridge, ipcRenderer } from 'electron';

/**
 * Exposes a minimal API to the renderer via contextBridge.
 * The renderer accesses this as window.wnwn.*
 *
 * The server URL is injected by the main process via executeJavaScript
 * after the page loads (window.__WNWN_SERVER_URL__). The renderer's
 * api/client.ts reads that value directly.
 */
contextBridge.exposeInMainWorld('wnwn', {
  /** Returns the current wnwn-server base URL. */
  getServerUrl: (): Promise<string> => ipcRenderer.invoke('get-server-url'),

  /** Platform string for platform-specific UI tweaks. */
  platform: process.platform,
});
