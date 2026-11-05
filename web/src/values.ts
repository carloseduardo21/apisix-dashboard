// Type-safe helpers for untyped data coming from the APISIX / Kong Admin APIs.
// Everything returned by those APIs is `unknown` until narrowed: use these
// guards instead of `as` assertions so malformed data cannot crash a screen or
// render as "[object Object]".
import { isObj, type Obj } from './schema/types'

export { isObj, type Obj }

/** The value as an object, or undefined when it is not a plain object. */
export const asObj = (v: unknown): Obj | undefined => (isObj(v) ? v : undefined)

/** Human-readable text for any value: strings as-is, scalars via String, objects as JSON. */
export function text(v: unknown): string {
  if (typeof v === 'string') return v
  if (v == null) return ''
  if (typeof v === 'number' || typeof v === 'boolean' || typeof v === 'bigint') return String(v)
  return JSON.stringify(v) ?? ''
}

/** A value that may be a single item or a list, as display strings. */
export const textList = (v: unknown): string[] => (Array.isArray(v) ? v : v == null ? [] : [v]).map(text)

export const isStringArray = (v: unknown): v is string[] => Array.isArray(v) && v.every((x) => typeof x === 'string')

/** String values of an object (e.g. labels), dropping non-string entries. */
export function stringRecord(v: unknown): Record<string, string> {
  const out: Record<string, string> = {}
  if (isObj(v)) for (const [k, x] of Object.entries(v)) if (typeof x === 'string') out[k] = x
  return out
}

export const numberOr = (v: unknown, fallback: number): number => (typeof v === 'number' ? v : fallback)

/** Message of a thrown value (fetch/ApiError/JSON.parse errors, or anything else). */
export const errorMessage = (e: unknown): string => (e instanceof Error ? e.message : text(e))
