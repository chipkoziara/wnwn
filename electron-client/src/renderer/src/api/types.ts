// Wire types matching the wnwn-server JSON API.
// Transitional bridge fields (Filename, SubgroupIx, etc.) are intentionally
// absent — the server omits them and clients only ever see stable IDs.

export interface Task {
  id: string;
  text: string;
  state: TaskState;
  created: string;        // RFC 3339
  modified_at?: string;   // RFC 3339
  deadline?: string;      // RFC 3339
  scheduled?: string;     // RFC 3339
  tags?: string[];
  url?: string;
  notes?: string;
  waiting_on?: string;
  waiting_since?: string; // RFC 3339
  source?: string;        // set when archived
  archived_at?: string;   // RFC 3339
}

export type TaskState =
  | ''               // inbox / unprocessed
  | 'next-action'
  | 'waiting-for'
  | 'some-day/maybe'
  | 'done'
  | 'canceled'
  | 'active';        // projects only

export interface TaskPatch {
  text?: string;
  state?: TaskState;
  deadline?: string | null;
  scheduled?: string | null;
  tags?: string[];
  url?: string;
  notes?: string;
  waiting_on?: string;
  clear?: string[];  // field names to clear, e.g. ["deadline", "tags"]
}

export type TaskLocationKind = 'inbox' | 'actions' | 'project' | 'archive';

export interface TaskLocation {
  kind: TaskLocationKind;
  task: Task;
  project_id?: string;
  subgroup_id?: string;
  archived?: boolean;
}

export interface SubGroup {
  id: string;
  title: string;
  state?: TaskState;
  deadline?: string;
  tasks: Task[];
}

export interface Project {
  id: string;
  title: string;
  state?: TaskState;
  deadline?: string;
  tags?: string[];
  url?: string;
  waiting_on?: string;
  definition_of_done?: string;
  sub_groups: SubGroup[];
}

export interface ProjectPatch {
  title?: string;
  state?: TaskState;
  deadline?: string | null;
  tags?: string[];
  url?: string;
  waiting_on?: string;
  definition_of_done?: string;
  clear?: string[];
}

export interface ProjectLocation {
  project_id: string;
  project: Project;
}

export interface ProjectSummary {
  id: string;
  title: string;
  state?: TaskState;
  deadline?: string;
  tags?: string[];
  sub_group_count: number;
  task_count: number;
  next_action?: string;
}

export interface ViewTask {
  task: Task;
  source_label: string;
  project_id?: string;
  is_project: boolean;
  is_archived: boolean;
}

export interface SavedView {
  name: string;
  query: string;
  include_archived: boolean;
}

export type InboxStep =
  | 'actionable'
  | 'not_actionable'
  | 'enrich'
  | 'route'
  | 'waiting_on'
  | 'new_project'
  | 'complete';

export interface InboxSessionItem {
  original: Task;
  draft: Task;
  step: InboxStep;
}

export interface InboxProgress {
  current: number;
  total: number;
}

export interface InboxSummary {
  trashed: number;
  someday: number;
  done: number;
  waiting: number;
  refiled: number;
  to_project: number;
  skipped: number;
}

export interface InboxSession {
  id: string;
  current: InboxSessionItem;
  progress: InboxProgress;
  summary: InboxSummary;
  done: boolean;
}

export type InboxDecisionKind =
  | 'trash'
  | 'done'
  | 'someday'
  | 'waiting'
  | 'single_action'
  | 'project'
  | 'new_project';

export interface InboxDecision {
  kind: InboxDecisionKind;
  waiting_on?: string;
  project_id?: string;
  project_title?: string;
}

export interface WeeklyReviewData {
  projects_without_next_action: ProjectSummary[];
  aging_waiting_for: ViewTask[];
  someday_maybe: ViewTask[];
  recent_archived: ViewTask[];
}

export interface ImportResult {
  mode: string;
  dry_run: boolean;
  inbox_added: number;
  actions_added: number;
  projects_added: number;
  archived_added: number;
  reset: boolean;
}
