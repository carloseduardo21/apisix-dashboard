// Thin client for the dashboard backend. The APISIX admin key never reaches
// the browser: everything goes through /api on the same origin.

let csrfToken = ''
let onUnauthorized: (() => void) | null = null

export function setCsrfToken(token: string) {
  csrfToken = token
}

export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn
}

export class ApiError extends Error {
  status: number
  body: unknown
  constructor(status: number, message: string, body: unknown) {
    super(message)
    this.status = status
    this.body = body
  }
}

function errorMessage(status: number, data: unknown): string {
  if (data && typeof data === 'object') {
    const d = data as Record<string, unknown>
    if (typeof d.error === 'string') return d.error
    if (typeof d.error_msg === 'string') return d.error_msg
    if (typeof d.message === 'string') return d.message
  }
  return `HTTP ${status}`
}

export async function api<T = unknown>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' }
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  if (method !== 'GET') headers['X-CSRF-Token'] = csrfToken

  const res = await fetch(path, {
    method,
    headers,
    credentials: 'same-origin',
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const text = await res.text()
  let data: unknown
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = text
    }
  }
  if (!res.ok) {
    if (res.status === 401 && !path.startsWith('/api/auth/')) onUnauthorized?.()
    throw new ApiError(res.status, errorMessage(res.status, data), data)
  }
  return data as T
}

// ---- types ----

export type Role = 'viewer' | 'operator' | 'admin'

export interface Me {
  username: string
  role: Role
  mfa_ok: boolean
  mfa_enrolled: boolean
  must_change_password: boolean
  csrf_token: string
  idle_timeout_seconds: number
  version: string
}

export interface AdminItem<T = Record<string, unknown>> {
  key: string
  value: T
  createdIndex?: number
  modifiedIndex?: number
}

export interface AdminList<T = Record<string, unknown>> {
  total: number
  list: AdminItem<T>[]
}

export interface DashboardUser {
  id: number
  username: string
  role: Role
  disabled: boolean
  locked: boolean
  mfa_enrolled: boolean
  must_change_password: boolean
  created_at: string
  last_login_at: string | null
}

export interface AuditEvent {
  id: number
  ts: string
  actor: string
  ip: string
  action: string
  resource?: string
  resource_id?: string
  outcome: 'success' | 'failure'
  detail?: string
  before?: unknown
  after?: unknown
}

export const MASKED = '******'

/** APISIX answers this when stream_proxy is not enabled in its config. */
export const isStreamDisabled = (e: unknown) => e instanceof Error && e.message.includes('stream mode is disabled')

export const canWrite = (role?: Role) => role === 'operator' || role === 'admin'
export const isAdmin = (role?: Role) => role === 'admin'

/** Id of an Admin API item, derived from its etcd key ("/apisix/routes/1" → "1"). */
export function itemId(resource: string, item: AdminItem): string {
  const prefix = `/apisix/${resource}/`
  return item.key.startsWith(prefix) ? item.key.slice(prefix.length) : item.key.split('/').pop() ?? ''
}
