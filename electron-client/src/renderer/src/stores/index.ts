import { writable, derived } from 'svelte/store';
import type { ServerEvent } from '../api/client';

// ---------------------------------------------------------------------------
// Navigation
// ---------------------------------------------------------------------------

export type ViewName = 'inbox' | 'actions' | 'projects' | 'project-detail' | 'views' | 'weekly-review';

export const currentView = writable<ViewName>('inbox');
export const activeProjectId = writable<string | null>(null);

export function navigate(view: ViewName, projectId?: string) {
  if (projectId !== undefined) activeProjectId.set(projectId);
  currentView.set(view);
}

// ---------------------------------------------------------------------------
// SSE event bus
// ---------------------------------------------------------------------------

// A simple writable that gets bumped whenever a server event arrives.
// Components that need to react to live updates can subscribe to this.
export const lastServerEvent = writable<ServerEvent | null>(null);

// Derived invalidation keys — components use these to decide when to refetch.
// Using a counter lets TanStack Query / manual fetch logic detect changes.
export const inboxVersion = writable(0);
export const actionsVersion = writable(0);
export const projectsVersion = writable(0);
export const viewsVersion = writable(0);

export function handleServerEvent(event: ServerEvent) {
  lastServerEvent.set(event);

  switch (event.type) {
    case 'task_created':
    case 'task_trashed':
    case 'task_archived':
    case 'task_restored':
    case 'task_moved':
      inboxVersion.update((n) => n + 1);
      actionsVersion.update((n) => n + 1);
      viewsVersion.update((n) => n + 1);
      if (event.project_id) projectsVersion.update((n) => n + 1);
      break;
    case 'task_updated':
      inboxVersion.update((n) => n + 1);
      actionsVersion.update((n) => n + 1);
      viewsVersion.update((n) => n + 1);
      break;
    case 'project_created':
    case 'project_updated':
    case 'subgroup_changed':
      projectsVersion.update((n) => n + 1);
      viewsVersion.update((n) => n + 1);
      break;
  }
}
