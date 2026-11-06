// Thin client for the dashboard backend. The APISIX admin key never reaches
// the browser: everything goes through /api on the same origin.
import { serverText } from './i18n/format'
import { isObj, text } from './values'

let csrfToken = ''
let onUnauthorized: (() => void) | null = null

// Requests sent while the user is not interacting (status polling, refetches)
// are marked as background so the backend does not extend the idle timeout:
// an unattended tab must reach the 15 min idle limit (PCI DSS 8.2.8).
const BACKGROUND_AFTER_MS = 30_000
let lastActivity = Date.now()
if (typeof window !== 'undefined') {
  const markActive = () => {
    lastActivity = Date.now()
  }
  for (const ev of ['pointerdown', 'keydown', 'wheel', 'touchstart'] as const) {
    window.addEventListener(ev, markActive, { capture: true, passive: true })
  }
}

/** Whether the user has been inactive long enough for requests to count as background. */
export function isUserIdle(now = Date.now()): boolean {
  return now - lastActivity > BACKGROUND_AFTER_MS
}

export function setCsrfToken(token: string) {
  csrfToken = token
}

export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn
}

export class ApiError extends Error {
  status: number
  body: unknown
  /** Stable error code from the backend (see internal/i18n), when present. */
  code?: string
  constructor(status: number, message: string, body: unknown, code?: string) {
    super(message)
    this.status = status
    this.body = body
    this.code = code
  }
}

const stringParams = (v: unknown): Record<string, string> | undefined =>
  isObj(v)
    ? Object.fromEntries(Object.entries(v).filter((e): e is [string, string] => typeof e[1] === 'string'))
    : undefined

/** A backend message in the current language: {error, code, params} or a list in "details". */
function messageOf(d: Record<string, unknown>): string | undefined {
  if (Array.isArray(d.details) && d.details.length > 0) {
    return d.details
      .filter(isObj)
      .map((m) => serverText({ code: text(m.code), params: stringParams(m.params) }))
      .join('\n')
  }
  if (typeof d.code === 'string') {
    return serverText({
      code: d.code,
      params: stringParams(d.params),
      message: typeof d.error === 'string' ? d.error : undefined,
    })
  }
  return undefined
}

function errorMessage(status: number, data: unknown): string {
  if (isObj(data)) {
    const translated = messageOf(data)
    if (translated) return translated
    // Errors relayed from the APISIX Admin API ({"error_msg": ...}) are English.
    if (typeof data.error === 'string') return data.error
    if (typeof data.error_msg === 'string') return data.error_msg
    if (typeof data.message === 'string') return data.message
  }
  return `HTTP ${status}`
}

export async function api<T = unknown>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' }
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  if (method !== 'GET') headers['X-CSRF-Token'] = csrfToken
  if (isUserIdle()) headers['X-Background'] = '1'

  const res = await fetch(path, {
    method,
    headers,
    credentials: 'same-origin',
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const raw = await res.text()
  let data: unknown
  if (raw) {
    try {
      data = JSON.parse(raw)
    } catch {
      data = raw
    }
  }
  if (!res.ok) {
    if (res.status === 401 && !path.startsWith('/api/auth/')) onUnauthorized?.()
    throw new ApiError(
      res.status,
      errorMessage(res.status, data),
      data,
      isObj(data) && typeof data.code === 'string' ? data.code : undefined,
    )
  }
  // oxlint-disable-next-line typescript/no-unsafe-type-assertion -- typed contract with our own backend; callers narrow APISIX payloads
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
  return item.key.startsWith(prefix) ? item.key.slice(prefix.length) : (item.key.split('/').pop() ?? '')
}
