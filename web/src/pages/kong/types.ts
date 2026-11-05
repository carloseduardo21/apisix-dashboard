import type { Obj } from '../../schema/types'

export type Kind = 'upstream' | 'service' | 'consumer' | 'credential' | 'route' | 'global_plugin'
export type Action = 'create' | 'update' | 'overwrite' | 'unchanged' | 'conflict' | 'blocked'

export interface Note {
  level: 'info' | 'warn' | 'error'
  message: string
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
  log: { ts: string; level: Note['level']; message: string }[]
  results: { kind: Kind; kong_name: string; path: string; action: Action; outcome: 'success' | 'failure' | 'rolled_back' | 'skipped'; error?: string }[]
}

export interface MigrationRecord {
  kind: Kind
  kong_id: string
  kong_name: string
  resource: string
  id: string
  migrated_by: string
  migrated_at: string
}

export const KIND_LABEL: Record<Kind, string> = {
  upstream: 'Upstream',
  service: 'Service',
  consumer: 'Consumer',
  credential: 'Credencial',
  route: 'Rota',
  global_plugin: 'Plugin global',
}

export const ACTION: Record<Action, { label: string; color: string; hint: string }> = {
  create: { label: 'criar', color: 'indigo', hint: 'Ainda não existe no APISIX' },
  update: { label: 'atualizar', color: 'yellow', hint: 'Migrado antes; o Kong ou os padrões mudaram' },
  overwrite: { label: 'sobrescrever', color: 'orange', hint: 'Já existe no APISIX sem ter vindo da migração; será substituído' },
  unchanged: { label: 'migrado', color: 'teal', hint: 'Já está no APISIX como no Kong' },
  conflict: { label: 'conflito', color: 'orange', hint: 'Já existe no APISIX sem ter vindo da migração; será pulado' },
  blocked: { label: 'bloqueado', color: 'red', hint: 'Não pode ser migrado; veja os avisos' },
}

export const writes = (a: Action) => a === 'create' || a === 'update' || a === 'overwrite'

/** Worst status of a group of items (a service with its upstream and routes). */
export function groupAction(items: PlanItem[]): Action {
  const order: Action[] = ['blocked', 'conflict', 'overwrite', 'update', 'create', 'unchanged']
  for (const a of order) if (items.some((i) => i.action === a)) return a
  return 'unchanged'
}

export const worstNote = (items: PlanItem[]) =>
  items.some((i) => i.notes.some((n) => n.level === 'error')) ? 'error' : items.some((i) => i.notes.some((n) => n.level === 'warn')) ? 'warn' : undefined
