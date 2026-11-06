import type { ServerMessage } from '../../i18n/format'
import type { Obj } from '../../schema/types'

export type Kind = 'upstream' | 'service' | 'consumer' | 'credential' | 'route' | 'global_plugin'
export type Action = 'create' | 'update' | 'overwrite' | 'unchanged' | 'conflict' | 'blocked'

/** A translation note from the backend (see internal/kong). message is English. */
export interface Note {
  level: 'info' | 'warn' | 'error'
  message: string
  code?: string
  params?: Record<string, string>
  plugin?: string
  reason?: { code?: string; params?: Record<string, string>; message?: string }
  /** UI only: which object of a group the note is about. */
  prefix?: string
}

export interface PluginMap {
  kong: string
  apisix?: string[]
  status: 'ok' | 'warn' | 'error' | 'ignored' | 'disabled'
  custom?: boolean
}

export interface PlanItem {
  kind: Kind
  kong_id: string
  kong_name: string
  detail?: string
  parent?: string
  resource: string
  id: string
  body?: Obj
  plugins?: PluginMap[]
  notes: Note[]
  blocked: boolean
  action: Action
  current?: Obj
  migrated_at?: string
  migrated_by?: string
  changed?: boolean
}

export interface Plan {
  items: PlanItem[]
  hash: string
}

export interface Selection {
  services: string[]
  routes: string[]
  consumers: string[]
  globals: string[]
}

export interface PluginRule {
  kong_plugin: string
  action: 'map' | 'ignore'
  apisix_plugin?: string
  template?: Obj
}

export interface Settings {
  id_prefix: string
  id_strategy: 'name' | 'kong_id'
  route_status: 'disabled' | 'enabled'
  on_conflict: 'skip' | 'overwrite'
  unsupported_plugins: 'block' | 'skip'
  disabled_plugins: 'skip' | 'keep'
  enable_websocket: boolean
  host_rewrites: { from: string; to: string }[]
  labels: Record<string, string>
  plugin_rules: PluginRule[]
}

export interface KongInfo {
  enabled: boolean
  url?: string
  info?: { version: string; hostname: string; database: string }
  error?: string
  code?: string
  params?: Record<string, string>
  settings?: Settings
  builtin_plugins?: string[]
}

export interface Job {
  id: string
  status: 'running' | 'success' | 'failed'
  actor: string
  started_at: string
  finished_at?: string
  total: number
  done: number
  error?: string
  error_message?: { code: string; params?: Record<string, string> }
  log: { ts: string; level: Note['level']; message: string; code?: string; params?: Record<string, string> }[]
  results: {
    kind: Kind
    kong_name: string
    path: string
    action: Action
    outcome: 'success' | 'failure' | 'rolled_back' | 'skipped'
    error?: string
  }[]
}

export interface MigrationRecord {
  record_id: number
  kind: Kind
  kong_id: string
  kong_name: string
  resource: string
  id: string
  migrated_by: string
  migrated_at: string
}

/** Translation keys of the object kinds: t(KIND_LABEL[kind]). */
export const KIND_LABEL = {
  upstream: 'kong.kind.upstream',
  service: 'kong.kind.service',
  consumer: 'kong.kind.consumer',
  credential: 'kong.kind.credential',
  route: 'kong.kind.route',
  global_plugin: 'kong.kind.global_plugin',
} as const satisfies Record<Kind, string>

/** Plan actions: color plus translation keys of the label and hint. */
export const ACTION = {
  create: { label: 'kong.action.create', color: 'indigo', hint: 'kong.action.create_hint' },
  update: { label: 'kong.action.update', color: 'yellow', hint: 'kong.action.update_hint' },
  overwrite: { label: 'kong.action.overwrite', color: 'orange', hint: 'kong.action.overwrite_hint' },
  unchanged: { label: 'kong.action.unchanged', color: 'teal', hint: 'kong.action.unchanged_hint' },
  conflict: { label: 'kong.action.conflict', color: 'orange', hint: 'kong.action.conflict_hint' },
  blocked: { label: 'kong.action.blocked', color: 'red', hint: 'kong.action.blocked_hint' },
} as const satisfies Record<Action, { label: string; color: string; hint: string }>

export const writes = (a: Action) => a === 'create' || a === 'update' || a === 'overwrite'

/** Worst status of a group of items (a service with its upstream and routes). */
export function groupAction(items: PlanItem[]): Action {
  const order: Action[] = ['blocked', 'conflict', 'overwrite', 'update', 'create', 'unchanged']
  for (const a of order) if (items.some((i) => i.action === a)) return a
  return 'unchanged'
}

export const worstNote = (items: PlanItem[]) =>
  items.some((i) => i.notes.some((n) => n.level === 'error'))
    ? 'error'
    : items.some((i) => i.notes.some((n) => n.level === 'warn'))
      ? 'warn'
      : undefined

export type RevertAction = 'delete' | 'restore' | 'manual' | 'missing'

export interface RevertItem {
  record_id: number
  kind: Kind
  kong_name: string
  path: string
  action: RevertAction
  /** Modified in APISIX after the migration: only reverted when included. */
  changed: boolean
  reason?: ServerMessage
}

export interface RevertPlan {
  items: RevertItem[]
  hash: string
}

export interface RevertResult {
  path: string
  kind: Kind
  action: RevertAction
  outcome: 'success' | 'failure' | 'skipped'
  error?: ServerMessage
}
