// Locale-aware formatting and translation of messages sent by the backend.
import i18n from './index'

/** A user-facing message from the API: a stable code, its params and the English text. */
export interface ServerMessage {
  code?: string
  params?: Record<string, string>
  message?: string
}

// Backend codes live under "server." in the catalogs. They are data, so the
// key is not known at compile time; go test ./internal/i18n guarantees every code the Go
// code emits exists in the catalogs.
// oxlint-disable-next-line typescript/no-unsafe-type-assertion -- dynamic keys are validated by go test ./internal/i18n
const translateDynamic = i18n.t.bind(i18n) as unknown as (key: string, options?: Record<string, unknown>) => string

/** Translates a backend message, falling back to its English text. */
export function serverText(m: ServerMessage | undefined, fallback = ''): string {
  if (!m) return fallback
  if (m.code && i18n.exists(`server.${m.code}`)) {
    const params: Record<string, unknown> = { ...m.params }
    // Numeric params enable plural forms ("count").
    for (const [k, v] of Object.entries(params)) if (typeof v === 'string' && /^-?\d+$/.test(v)) params[k] = Number(v)
    return translateDynamic(`server.${m.code}`, params)
  }
  return m.message ?? fallback
}

/** A note of the Kong translator: optional plugin prefix and nested reason. */
export interface NoteLike extends ServerMessage {
  plugin?: string
  reason?: ServerMessage
}

export function noteText(n: NoteLike): string {
  const main = serverText(n)
  const body = n.reason ? `${serverText(n.reason)} (${main})` : main
  return n.plugin ? `${n.plugin}: ${body}` : body
}

const date = (v: string | number | Date) => (v instanceof Date ? v : new Date(v))

export const formatDateTime = (v: string | number | Date) => date(v).toLocaleString(i18n.language)
export const formatDate = (v: string | number | Date) => date(v).toLocaleDateString(i18n.language)
export const formatTime = (v: string | number | Date) => date(v).toLocaleTimeString(i18n.language)

/** Translates a key built from runtime data (e.g. an audit action), or returns the fallback. */
export function dynamicText(key: string, fallback: string, params?: Record<string, unknown>): string {
  return i18n.exists(key) ? translateDynamic(key, params) : fallback
}
