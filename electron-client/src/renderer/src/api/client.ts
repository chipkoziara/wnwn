/**
 * Typed HTTP client for the wnwn-server API.
 *
 * The server base URL is injected by the Electron main process via
 * executeJavaScript after page load (window.__WNWN_SERVER_URL__).
 * In the Vite dev server we fall back to a default for easier iteration.
 */

import type {
  Task,
  TaskPatch,
  TaskLocation,
  Project,
  ProjectPatch,
  ProjectLocation,
  ProjectSummary,
  ViewTask,
  SavedView,
  InboxSession,
  InboxDecision,
  WeeklyReviewData,
  ImportResult,
} from './types';

function baseUrl(): string {
  // Injected by main process; falls back to default for dev server testing.
  return (window as any).__WNWN_SERVER_URL__ ?? 'http://127.0.0.1:9274';
}

// ---------------------------------------------------------------------------
// HTTP primitives
// ---------------------------------------------------------------------------

async function request<T>(
  method: string,
  path: string,
  body?: unknown
): Promise<T> {
  const res = await fetch(`${baseUrl()}${path}`, {
    method,
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : {},
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });

  const json = await res.json();

  if (json.error) {
    throw new Error(json.error);
  }

  return json.data as T;
}

const get = <T>(path: string) => request<T>('GET', path);
const post = <T>(path: string, body?: unknown) => request<T>('POST', path, body ?? null);
const patch = <T>(path: string, body: unknown) => request<T>('PATCH', path, body);
const del = <T>(path: string) => request<T>('DELETE', path);

// ---------------------------------------------------------------------------
// Tasks
// ---------------------------------------------------------------------------

/** Lists tasks. Pass list='in' or 'single-actions' for a specific list,
 *  or omit for all active tasks. Pass archived=true to include archives. */
export function listTasks(opts?: { list?: 'in' | 'single-actions'; archived?: boolean }) {
  const params = new URLSearchParams();
  if (opts?.list) params.set('list', opts.list);
  if (opts?.archived) params.set('archived', 'true');
  const qs = params.toString();
  return get<ViewTask[] | Task[]>(`/api/tasks${qs ? '?' + qs : ''}`);
}

export function listInbox() {
  return get<Task[]>('/api/tasks?list=in');
}

export function listActions() {
  return get<Task[]>('/api/tasks?list=single-actions');
}

export function captureToInbox(body: {
  text: string;
  deadline?: string;
  scheduled?: string;
  tags?: string[];
  url?: string;
  notes?: string;
  waiting_on?: string;
}) {
  return post<{ task: Task }>('/api/inbox', body);
}

export function getTask(id: string) {
  return get<{ location: TaskLocation }>(`/api/tasks/${id}`);
}

export function updateTask(id: string, patch_: TaskPatch) {
  return patch<{ location: TaskLocation }>(`/api/tasks/${id}`, patch_);
}

export function trashTask(id: string) {
  return del<{ trashed: boolean }>(`/api/tasks/${id}`);
}

export function archiveTask(id: string) {
  return post<{ archived: boolean }>(`/api/tasks/${id}/archive`);
}

export function restoreTask(id: string) {
  return post<{ location: TaskLocation }>(`/api/tasks/${id}/restore`);
}

export function moveTaskToList(
  id: string,
  list: 'in' | 'single-actions',
  state?: string
) {
  return post<{ location: TaskLocation }>(`/api/tasks/${id}/move`, {
    to: 'list',
    list,
    state: state ?? 'next-action',
  });
}

export function moveTaskToProject(
  id: string,
  projectId: string,
  subgroupId: string,
  state?: string
) {
  return post<{ location: TaskLocation }>(`/api/tasks/${id}/move`, {
    to: 'project',
    project_id: projectId,
    subgroup_id: subgroupId,
    state: state ?? 'next-action',
  });
}

export function moveTaskToSubgroup(id: string, subgroupId: string) {
  return post<{ moved: boolean }>(`/api/tasks/${id}/move-subgroup`, {
    subgroup_id: subgroupId,
  });
}

export function reorderTask(id: string, delta: 1 | -1) {
  return post<{ reordered: boolean }>(`/api/tasks/${id}/reorder`, { delta });
}

// ---------------------------------------------------------------------------
// Projects
// ---------------------------------------------------------------------------

export function listProjects() {
  return get<{ projects: ProjectSummary[] }>('/api/projects');
}

export function createProject(title: string, subgroupTitle = 'Tasks') {
  return post<{ location: ProjectLocation }>('/api/projects', {
    title,
    subgroup_title: subgroupTitle,
  });
}

export function getProject(id: string) {
  return get<{ location: ProjectLocation }>(`/api/projects/${id}`);
}

export function updateProject(id: string, patch_: ProjectPatch) {
  return patch<{ location: ProjectLocation }>(`/api/projects/${id}`, patch_);
}

// ---------------------------------------------------------------------------
// Subgroups
// ---------------------------------------------------------------------------

export function createSubgroup(projectId: string, title: string) {
  return post<{ project_id: string; subgroup_id: string; title: string }>(
    `/api/projects/${projectId}/subgroups`,
    { title }
  );
}

export function renameSubgroup(projectId: string, subgroupId: string, title: string) {
  return patch<{ project_id: string; subgroup_id: string; title: string }>(
    `/api/projects/${projectId}/subgroups/${subgroupId}`,
    { title }
  );
}

export function deleteSubgroup(projectId: string, subgroupId: string) {
  return del<{ deleted: boolean }>(
    `/api/projects/${projectId}/subgroups/${subgroupId}`
  );
}

export function addProjectTask(
  projectId: string,
  subgroupId: string,
  body: {
    text: string;
    deadline?: string;
    scheduled?: string;
    tags?: string[];
    url?: string;
    notes?: string;
    waiting_on?: string;
  }
) {
  return post<{ location: TaskLocation }>(
    `/api/projects/${projectId}/subgroups/${subgroupId}/tasks`,
    body
  );
}

// ---------------------------------------------------------------------------
// Views & queries
// ---------------------------------------------------------------------------

export function listViews() {
  return get<{ views: SavedView[] }>('/api/views');
}

export function runView(name: string) {
  return get<{ tasks: ViewTask[] }>(`/api/views/${encodeURIComponent(name)}/run`);
}

export function runQuery(query: string, includeArchived = false) {
  return post<{ tasks: ViewTask[] }>('/api/query', {
    query,
    include_archived: includeArchived,
  });
}

export function queryProjects(query: string) {
  return post<{ projects: ProjectLocation[] }>('/api/query/projects', { query });
}

// ---------------------------------------------------------------------------
// Weekly review
// ---------------------------------------------------------------------------

export function weeklyReview() {
  return get<WeeklyReviewData>('/api/review/weekly');
}

// ---------------------------------------------------------------------------
// Process Inbox sessions
// ---------------------------------------------------------------------------

export function startInboxSession() {
  return post<{ session: InboxSession }>('/api/inbox-sessions');
}

export function getInboxSession(id: string) {
  return get<{ session: InboxSession }>(`/api/inbox-sessions/${id}`);
}

export function updateInboxDraft(id: string, patch_: TaskPatch) {
  return patch<{ session: InboxSession }>(`/api/inbox-sessions/${id}/draft`, patch_);
}

export function commitInboxDecision(id: string, decision: InboxDecision) {
  return post<{ session: InboxSession }>(`/api/inbox-sessions/${id}/decide`, decision);
}

export function skipInboxItem(id: string) {
  return post<{ session: InboxSession }>(`/api/inbox-sessions/${id}/skip`);
}

export function discardInboxSession(id: string) {
  return del<{ discarded: boolean }>(`/api/inbox-sessions/${id}`);
}

// ---------------------------------------------------------------------------
// Import / export
// ---------------------------------------------------------------------------

export function exportMarkdown(outputDir: string) {
  return post<{ exported: boolean; output_dir: string }>('/api/export-md', {
    output_dir: outputDir,
  });
}

export function importMarkdown(opts: {
  dir: string;
  mode?: 'merge' | 'replace';
  dryRun?: boolean;
}) {
  return post<ImportResult>('/api/import-md', {
    dir: opts.dir,
    mode: opts.mode ?? 'merge',
    dry_run: opts.dryRun ?? false,
  });
}

// ---------------------------------------------------------------------------
// SSE live updates
// ---------------------------------------------------------------------------

export interface ServerEvent {
  type: string;
  task_id?: string;
  project_id?: string;
}

/**
 * Connects to the SSE event stream.
 * Returns an EventSource so the caller can close it.
 * Automatically reconnects with 2s backoff on error.
 */
export function connectEvents(onEvent: (e: ServerEvent) => void): EventSource {
  const source = new EventSource(`${baseUrl()}/api/events`);

  source.onmessage = (msg) => {
    try {
      const event = JSON.parse(msg.data) as ServerEvent;
      onEvent(event);
    } catch {
      // Ignore non-JSON messages (e.g. keep-alive comments).
    }
  };

  return source;
}
