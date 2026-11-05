import type { ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Accordion, Select, TagsInput, Text, TextInput, Textarea } from '@mantine/core'
import { api, itemId, type AdminList } from '../api'
import { KeyValueEditor, ObjectFields } from '../schema/SchemaForm'
import { isObj, setKey, useCatalog, type Obj } from '../schema/types'

export interface FormProps {
  value: Obj
  onChange: (v: Obj) => void
  isNew: boolean
}

// Managed by APISIX or by the editor itself; never shown in "advanced".
export const SYSTEM_FIELDS = ['id', 'create_time', 'update_time', 'validity_start', 'validity_end']

export const str = (v: unknown) => (typeof v === 'string' ? v : '')

/** Binds a top-level text field. */
export function textProps(value: Obj, onChange: (v: Obj) => void, key: string) {
  return {
    value: str(value[key]),
    onChange: (e: { currentTarget: { value: string } }) => onChange(setKey(value, key, e.currentTarget.value)),
  }
}

export function NameDesc({ value, onChange, nameLabel = 'Nome' }: { value: Obj; onChange: (v: Obj) => void; nameLabel?: string }) {
  return (
    <>
      <TextInput label={nameLabel} {...textProps(value, onChange, 'name')} />
      <Textarea label="Descrição" autosize minRows={1} maxRows={4} {...textProps(value, onChange, 'desc')} />
    </>
  )
}

/**
 * Edits fields that accept either a single value or a list (uri/uris,
 * host/hosts, sni/snis), writing the single form when there is one item.
 */
export function OneOrMany({
  value,
  onChange,
  single,
  many,
  ...rest
}: {
  value: Obj
  onChange: (v: Obj) => void
  single: string
  many: string
  label: string
  description?: string
  placeholder?: string
  required?: boolean
}) {
  const list = Array.isArray(value[many]) ? (value[many] as string[]) : typeof value[single] === 'string' ? [value[single]] : []
  return (
    <TagsInput
      label={rest.label}
      description={rest.description}
      placeholder={rest.placeholder}
      withAsterisk={rest.required}
      value={list}
      clearable
      onChange={(next) => {
        let out = setKey(setKey(value, single, undefined), many, undefined)
        if (next.length === 1) out = setKey(out, single, next[0])
        else if (next.length > 1) out = setKey(out, many, next)
        onChange(out)
      }}
    />
  )
}

export function useResourceOptions(resource: string) {
  return useQuery({
    queryKey: ['apisix', resource],
    queryFn: () => api<AdminList>('GET', `/api/apisix/${resource}`),
    select: (data) =>
      (data.list ?? []).map((item) => {
        const id = itemId(resource, item)
        const name = str(item.value.name) || str(item.value.username)
        return { value: id, label: name && name !== id ? `${name} (${id})` : id }
      }),
  })
}

export function ResourceSelect({
  resource,
  label,
  value,
  onChange,
  description,
  required,
}: {
  resource: string
  label: string
  value: unknown
  onChange: (v: string | undefined) => void
  description?: string
  required?: boolean
}) {
  const { data = [], isPending } = useResourceOptions(resource)
  const current = value === undefined || value === null ? null : String(value)
  // Keep a dangling reference visible instead of silently clearing it.
  const options = current && !data.some((o) => o.value === current) ? [...data, { value: current, label: `${current} (não encontrado)` }] : data
  return (
    <Select
      label={label}
      description={description}
      withAsterisk={required}
      data={options}
      value={current}
      onChange={(v) => onChange(v ?? undefined)}
      searchable
      clearable={!required}
      placeholder={isPending ? 'Carregando...' : 'Selecione'}
      nothingFoundMessage="Nada encontrado"
      comboboxProps={{ withinPortal: true }}
    />
  )
}

export function LabelsField({ value, onChange }: { value: Obj; onChange: (v: Obj) => void }) {
  return (
    <KeyValueEditor
      label="Labels"
      description="Pares chave/valor para organizar e filtrar recursos"
      value={isObj(value.labels) ? (value.labels as Record<string, string>) : undefined}
      onChange={(v) => onChange(setKey(value, 'labels', v))}
    />
  )
}

/** Renders every schema property the form did not handle explicitly. */
export function AdvancedFields({
  schemaName,
  value,
  onChange,
  handled,
}: {
  schemaName: string
  value: Obj
  onChange: (v: Obj) => void
  handled: string[]
}) {
  const catalog = useCatalog()
  const schema = catalog.data?.resources[schemaName]
  if (!schema?.properties) return null
  const exclude = [...SYSTEM_FIELDS, ...handled]
  const remaining = Object.keys(schema.properties).filter((k) => !exclude.includes(k))
  if (!remaining.length) return null
  return (
    <ObjectFields
      schema={{ ...schema, required: [] }}
      value={value}
      onChange={onChange}
      path=""
      exclude={exclude}
      moreLabel="Campos"
    />
  )
}

/** Accordion section wrapper used by every resource form. */
export function Section({ value, title, hint, children }: { value: string; title: string; hint?: ReactNode; children: ReactNode }) {
  return (
    <Accordion.Item value={value}>
      <Accordion.Control>
        <Text fw={500}>{title}</Text>
        {hint && (
          <Text size="xs" c="dimmed">
            {hint}
          </Text>
        )}
      </Accordion.Control>
      <Accordion.Panel>{children}</Accordion.Panel>
    </Accordion.Item>
  )
}
