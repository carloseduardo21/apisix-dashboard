import { useQuery } from '@tanstack/react-query'
import { api } from '../api'

/** The subset of JSON Schema (draft 4/7) used by APISIX. */
export interface JSONSchema {
  type?: string
  properties?: Record<string, JSONSchema>
  patternProperties?: Record<string, JSONSchema>
  additionalProperties?: boolean | JSONSchema
  required?: string[]
  items?: JSONSchema | JSONSchema[]
  enum?: unknown[]
  const?: unknown
  default?: unknown
  description?: string
  title?: string
  minimum?: number
  maximum?: number
  exclusiveMinimum?: number | boolean
  minLength?: number
  maxLength?: number
  pattern?: string
  minItems?: number
  maxItems?: number
  oneOf?: JSONSchema[]
  anyOf?: JSONSchema[]
  allOf?: JSONSchema[]
  encrypt_fields?: string[]
}

export interface PluginInfo {
  priority: number
  version?: number
  type?: string
  scope?: string
  schema: JSONSchema
  consumer_schema?: JSONSchema
  metadata_schema?: JSONSchema
}

export interface Catalog {
  plugins: Record<string, PluginInfo>
  stream_plugins: Record<string, PluginInfo>
  resources: Record<string, JSONSchema>
}

/** Plugin and resource schemas published by APISIX (cached by the backend). */
export function useCatalog() {
  return useQuery({
    queryKey: ['catalog'],
    queryFn: () => api<Catalog>('GET', '/api/apisix/schemas'),
    staleTime: 10 * 60_000,
  })
}

export type Obj = Record<string, unknown>

export const isObj = (v: unknown): v is Obj => !!v && typeof v === 'object' && !Array.isArray(v)

function isEmpty(v: unknown) {
  return v === undefined || v === '' || (Array.isArray(v) && v.length === 0) || (isObj(v) && Object.keys(v).length === 0)
}

/** Returns a copy of obj with key set, or removed when v is empty. */
export function setKey(obj: Obj | undefined, key: string, v: unknown): Obj {
  const out = { ...obj }
  if (isEmpty(v)) delete out[key]
  else out[key] = v
  return out
}

/** Resolves allOf and fills in the type when it is implied. */
export function normalize(s: JSONSchema): JSONSchema {
  let out = s
  if (s.allOf?.length) {
    out = { ...s, allOf: undefined }
    for (const part of s.allOf) {
      out = {
        ...out,
        ...part,
        properties: { ...out.properties, ...part.properties },
        required: [...(out.required ?? []), ...(part.required ?? [])],
      }
    }
  }
  if (!out.type) {
    if (out.properties || out.patternProperties) return { ...out, type: 'object' }
    if (out.items) return { ...out, type: 'array' }
    if (out.enum?.length) return { ...out, type: typeof out.enum[0] }
  }
  return out
}

/** A oneOf/anyOf branch that only adds constraints (e.g. {required: [...]}) and renders nothing. */
export function isConstraintOnly(s: JSONSchema) {
  return !s.type && !s.properties && !s.items && !s.enum && s.const === undefined && !s.oneOf && !s.anyOf && !s.patternProperties
}

export function valueType(v: unknown): string | undefined {
  if (v === undefined || v === null) return undefined
  if (Array.isArray(v)) return 'array'
  if (typeof v === 'number') return Number.isInteger(v) ? 'integer' : 'number'
  return typeof v
}

/** Picks the oneOf/anyOf branch that best matches the current value. */
export function matchVariant(variants: JSONSchema[], value: unknown): number {
  const vt = valueType(value)
  if (vt === undefined) return 0
  let best = 0
  let bestScore = -1
  variants.forEach((raw, i) => {
    const v = normalize(raw)
    let score = 0
    if (v.type === vt || (v.type === 'number' && vt === 'integer')) score += 10
    if (v.const !== undefined && v.const === value) score += 20
    if (v.enum?.includes(value)) score += 20
    if (isObj(value) && v.properties) {
      for (const k of Object.keys(value)) if (k in v.properties) score += 2
      for (const k of v.required ?? []) score += k in value ? 3 : -5
    }
    if (score > bestScore) {
      best = i
      bestScore = score
    }
  })
  return best
}

export function variantLabel(raw: JSONSchema, i: number): string {
  const v = normalize(raw)
  if (v.title) return v.title
  if (v.const !== undefined) return String(v.const)
  if (v.required?.length && v.type === 'object') return v.required.join(' + ')
  if (v.description && v.description.length < 40) return v.description
  const names: Record<string, string> = {
    string: 'texto',
    integer: 'número inteiro',
    number: 'número',
    boolean: 'sim/não',
    object: 'objeto',
    array: 'lista',
  }
  return v.type ? names[v.type] ?? v.type : `Opção ${i + 1}`
}

// Field names that hold secrets in any plugin (mirrors the backend rules).
const SENSITIVE_NAMES = new Set([
  'password',
  'redis_password',
  'secret',
  'secret_key',
  'private_key',
  'client_secret',
  'client_key',
  'secret_access_key',
  'session_token',
  'token',
  'api_key',
  'access_token',
])

/** Builds a predicate telling whether a field path (dot separated, array indexes omitted) holds a secret. */
export function sensitiveMatcher(encryptFields: string[] = [], extra: string[] = []) {
  const exact = new Set([...encryptFields, ...extra])
  return (path: string) => {
    if (exact.has(path)) return true
    for (const f of exact) if (path.startsWith(f + '.')) return true
    return SENSITIVE_NAMES.has(path.split('.').pop() ?? '')
  }
}
