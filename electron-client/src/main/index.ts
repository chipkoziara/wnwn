import { app, BrowserWindow, dialog, ipcMain } from 'electron';
import * as path from 'path';
import { startServer, stopServer } from './server-manager';

let mainWindow: BrowserWindow | null = null;
let serverUrl: string | null = null;

const isDev = process.env.NODE_ENV === 'development' || !app.isPackaged;

async function createWindow(url: string): Promise<void> {
  mainWindow = new BrowserWindow({
    width: 1280,
    height: 800,
    minWidth: 900,
    minHeight: 600,
    titleBarStyle: process.platform === 'darwin' ? 'hiddenInset' : 'default',
    webPreferences: {
      preload: path.join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false, // needed for preload to access process.env
    },
  });

  // Inject the server URL into the renderer environment via a env var
  // that the preload script reads before contextBridge setup.
  mainWindow.webContents.on('did-finish-load', () => {
    mainWindow?.webContents.executeJavaScript(
      `window.__WNWN_SERVER_URL__ = ${JSON.stringify(url)};`
    );
  });

  if (isDev) {
    // In dev mode the Vite dev server runs the renderer.
    await mainWindow.loadURL('http://localhost:5173');
    mainWindow.webContents.openDevTools();
  } else {
    await mainWindow.loadFile(
      path.join(__dirname, '../../dist/renderer/index.html')
    );
  }

  mainWindow.on('closed', () => {
    mainWindow = null;
  });
}

app.whenReady().then(async () => {
  try {
    serverUrl = await startServer(onServerCrash);
    await createWindow(serverUrl);
  } catch (err) {
    dialog.showErrorBox(
      'wnwn failed to start',
      `Could not start the wnwn server:\n\n${err}`
    );
    app.quit();
  }

  app.on('activate', async () => {
    // macOS: re-open window when dock icon clicked and no windows are open.
    if (BrowserWindow.getAllWindows().length === 0 && serverUrl) {
      await createWindow(serverUrl);
    }
  });
});

app.on('window-all-closed', () => {
  // On macOS, apps stay in the dock even with all windows closed.
  if (process.platform !== 'darwin') {
    app.quit();
  }
});

app.on('will-quit', () => {
  stopServer();
});

// IPC handler so renderer can request the server URL if needed.
ipcMain.handle('get-server-url', () => serverUrl);

let appIsQuitting = false;
app.on('will-quit', () => { appIsQuitting = true; });

function onServerCrash(code: number | null): void {
  if (appIsQuitting) return;

  const choice = dialog.showMessageBoxSync({
    type: 'error',
    title: 'wnwn server stopped',
    message: 'The wnwn data server has stopped unexpectedly.',
    detail: `Exit code: ${code ?? 'unknown'}\n\nWould you like to restart it?`,
    buttons: ['Restart', 'Quit'],
    defaultId: 0,
    cancelId: 1,
  });

  if (choice === 0) {
    startServer(onServerCrash)
      .then((url) => {
        serverUrl = url;
        // Reload all open windows with the new server URL.
        for (const win of BrowserWindow.getAllWindows()) {
          win.webContents.executeJavaScript(
            `window.__WNWN_SERVER_URL__ = ${JSON.stringify(url)};`
          );
          win.reload();
        }
      })
      .catch((err) => {
        dialog.showErrorBox('Restart failed', String(err));
        app.quit();
      });
  } else {
    app.quit();
  }
}
