import { useEffect, useRef, useState, type ReactNode } from 'react'
import {
  ActionIcon,
  Box,
  Button,
  Collapse,
  Fieldset,
  Group,
  Input,
  JsonInput,
  MultiSelect,
  NumberInput,
  Paper,
  PasswordInput,
  SegmentedControl,
  Select,
  Stack,
  Switch,
  TagsInput,
  Text,
  TextInput,
  Textarea,
  UnstyledButton,
} from '@mantine/core'
import { IconChevronDown, IconChevronRight, IconPlus, IconTrash } from '@tabler/icons-react'
import {
  isConstraintOnly,
  isObj,
  matchVariant,
  normalize,
  setKey,
  variantLabel,
  type JSONSchema,
  type Obj,
} from './types'
import { text } from '../values'

export interface FieldProps {
  schema: JSONSchema
  value: unknown
  onChange: (v: unknown) => void
  name: string
  required?: boolean
  /** Dot separated path from the form root; array indexes are omitted. */
  path: string
  sensitive?: (path: string) => boolean
}

const MASKED = '******'

function defaultText(s: JSONSchema) {
  if (s.default === undefined) return undefined
  return text(s.default)
}

function describe(s: JSONSchema): string | undefined {
  const parts: string[] = []
  if (s.description) parts.push(s.description)
  const d = defaultText(s)
  if (d !== undefined && s.type !== 'boolean') parts.push(`padrão: ${d}`)
  if (s.minimum !== undefined || s.maximum !== undefined) {
    parts.push(`faixa: ${s.minimum ?? '−∞'} – ${s.maximum ?? '∞'}`)
  }
  return parts.length ? parts.join(' · ') : undefined
}

const NO_EXCLUDE: string[] = []

/** String/number entries of a map value (other types are not editable here). */
function scalarRecord(v: Obj): Record<string, string | number> {
  const out: Record<string, string | number> = {}
  for (const [k, x] of Object.entries(v)) if (typeof x === 'string' || typeof x === 'number') out[k] = x
  return out
}

/** Renders one value according to its schema. */
export function SchemaField(props: FieldProps) {
  const schema = normalize(props.schema)
  const variants = (schema.oneOf ?? schema.anyOf ?? []).filter((v) => !isConstraintOnly(v))

  if (variants.length > 1) return <VariantField {...props} schema={schema} variants={variants} />
  if (variants.length === 1 && !schema.type) {
    return <SchemaField {...props} schema={{ ...schema, oneOf: undefined, anyOf: undefined, ...variants[0] }} />
  }

  const p = { ...props, schema }
  if (schema.const !== undefined) return <TextInput label={props.name} value={text(schema.const)} disabled />
  if (schema.enum) return <EnumField {...p} />
  switch (schema.type) {
    case 'string':
      return <StringField {...p} />
    case 'integer':
    case 'number':
      return <NumberField {...p} />
    case 'boolean':
      return <BooleanField {...p} />
    case 'array':
      return <ArrayField {...p} />
    case 'object':
      return <ObjectField {...p} />
  }
  return <JsonField {...p} />
}

function VariantField(props: FieldProps & { variants: JSONSchema[] }) {
  const { variants, value, onChange, name, schema } = props
  const [selected, setSelected] = useState(() => matchVariant(variants, value))
  const base: JSONSchema = { ...schema, oneOf: undefined, anyOf: undefined }
  const current = normalize({
    ...base,
    ...variants[selected],
    properties: { ...base.properties, ...variants[selected].properties },
  })
  return (
    <Stack gap={4}>
      <Group gap={6} wrap="nowrap">
        <Text size="xs" c="dimmed">
          {name ? `${name}: formato` : 'formato'}
        </Text>
        <SegmentedControl
          size="xs"
          value={String(selected)}
          onChange={(v) => {
            setSelected(Number(v))
            onChange(undefined)
          }}
          data={variants.map((v, i) => ({ value: String(i), label: variantLabel(v, i) }))}
        />
      </Group>
      <SchemaField {...props} schema={current} />
    </Stack>
  )
}

function EnumField({ schema, value, onChange, name, required }: FieldProps) {
  const options = (schema.enum ?? []).map((e) => ({ value: JSON.stringify(e), label: String(e) }))
  return (
    <Select
      label={name || undefined}
      description={describe(schema)}
      withAsterisk={required}
      data={options}
      value={value === undefined ? null : JSON.stringify(value)}
      placeholder={defaultText(schema)}
      clearable={!required}
      onChange={(v) => onChange(v === null ? undefined : JSON.parse(v))}
      comboboxProps={{ withinPortal: true }}
    />
  )
}

function StringField({ schema, value, onChange, name, required, path, sensitive }: FieldProps) {
  const str = text(value)
  const common = {
    label: name || undefined,
    description: describe(schema),
    withAsterisk: required,
    placeholder: defaultText(schema),
    value: str,
  }
  if (sensitive?.(path)) {
    return (
      <PasswordInput
        {...common}
        description={
          str === MASKED ? 'Valor atual mascarado. Mantenha para preservar ou digite um novo.' : common.description
        }
        onChange={(e) => onChange(e.currentTarget.value)}
      />
    )
  }
  if ((schema.maxLength ?? 0) > 1024 || str.includes('\n')) {
    return (
      <Textarea
        {...common}
        autosize
        minRows={2}
        maxRows={12}
        styles={{ input: { fontFamily: 'var(--mantine-font-family-monospace)' } }}
        onChange={(e) => onChange(e.currentTarget.value)}
      />
    )
  }
  return <TextInput {...common} onChange={(e) => onChange(e.currentTarget.value)} />
}

function NumberField({ schema, value, onChange, name, required }: FieldProps) {
  const min = typeof schema.exclusiveMinimum === 'number' ? schema.exclusiveMinimum : schema.minimum
  return (
    <NumberInput
      label={name || undefined}
      description={describe(schema)}
      withAsterisk={required}
      placeholder={defaultText(schema)}
      value={typeof value === 'number' ? value : ''}
      min={min}
      max={schema.maximum}
      allowDecimal={schema.type === 'number'}
      onChange={(v) => onChange(typeof v === 'number' ? v : v === '' ? undefined : Number(v))}
    />
  )
}

function BooleanField({ schema, value, onChange, name }: FieldProps) {
  const checked = typeof value === 'boolean' ? value : schema.default === true
  return (
    <Switch
      label={name}
      description={describe(schema)}
      checked={checked}
      onChange={(e) => onChange(e.currentTarget.checked)}
    />
  )
}

function ArrayField(props: FieldProps) {
  const { schema, value, onChange, name, required, path } = props
  const items = normalize((Array.isArray(schema.items) ? schema.items[0] : schema.items) ?? {})
  const list = Array.isArray(value) ? value : []

  if (items.enum) {
    const enumValues = items.enum
    return (
      <MultiSelect
        label={name}
        description={describe(schema)}
        withAsterisk={required}
        data={items.enum.map(text)}
        value={list.map(text)}
        onChange={(v) => onChange(v.map((s) => enumValues.find((e) => text(e) === s)))}
        searchable
        clearable
      />
    )
  }
  if (items.type === 'string' || items.type === 'integer' || items.type === 'number') {
    const numeric = items.type !== 'string'
    return (
      <TagsInput
        label={name}
        description={[describe(schema), 'Enter para adicionar'].filter(Boolean).join(' · ')}
        withAsterisk={required}
        value={list.map(String)}
        onChange={(v) => onChange(numeric ? v.map(Number).filter((n) => !Number.isNaN(n)) : v)}
        clearable
      />
    )
  }
  if (items.type === 'object' && items.properties) {
    return (
      <Input.Wrapper label={name} description={describe(schema)} withAsterisk={required}>
        <Stack gap="xs" mt={4}>
          {list.map((item, i) => (
            // oxlint-disable-next-line react/no-array-index-key -- array items have no identity; fields are controlled
            <Paper key={i} withBorder p="sm" radius="sm">
              <Group justify="space-between" mb={4}>
                <Text size="xs" c="dimmed">
                  #{i + 1}
                </Text>
                <ActionIcon
                  size="sm"
                  variant="subtle"
                  color="red"
                  onClick={() => onChange(list.filter((_, j) => j !== i))}
                >
                  <IconTrash size={14} />
                </ActionIcon>
              </Group>
              <ObjectFields
                schema={items}
                value={item}
                path={path}
                sensitive={props.sensitive}
                onChange={(v) => onChange(list.map((x, j) => (j === i ? v : x)))}
              />
            </Paper>
          ))}
          <Button
            size="xs"
            variant="light"
            leftSection={<IconPlus size={14} />}
            onClick={() => onChange([...list, {}])}
            w="fit-content"
          >
            Adicionar
          </Button>
        </Stack>
      </Input.Wrapper>
    )
  }
  return <JsonField {...props} />
}

function ObjectField(props: FieldProps) {
  const { schema, name, required, path } = props
  const mapSchema = schema.patternProperties
    ? Object.values(schema.patternProperties)[0]
    : isObj(schema.additionalProperties)
      ? schema.additionalProperties
      : undefined

  if (!schema.properties && mapSchema) {
    const valueType = normalize(mapSchema).type
    if (valueType === 'string' || valueType === 'integer' || valueType === 'number') {
      return <KeyValueField {...props} numeric={valueType !== 'string'} />
    }
  }
  if (!schema.properties) return <JsonField {...props} />

  const body = (
    <ObjectFields
      schema={schema}
      value={props.value}
      onChange={props.onChange}
      path={path}
      sensitive={props.sensitive}
    />
  )
  if (!name) return body
  return (
    <Fieldset
      legend={
        <Text size="sm" fw={500}>
          {name}
          {required && (
            <Text span c="red">
              {' '}
              *
            </Text>
          )}
        </Text>
      }
      variant="filled"
      p="sm"
    >
      {schema.description && (
        <Text size="xs" c="dimmed" mb="xs">
          {schema.description}
        </Text>
      )}
      {body}
    </Fieldset>
  )
}

/**
 * Fields required only in some configurations (root oneOf/anyOf branches or
 * if/then/else), e.g. limit-count's count and time_window. They are shown
 * up front, without the required marker.
 */
function conditionallyRequired(s: JSONSchema & { then?: JSONSchema; else?: JSONSchema }): Set<string> {
  const out = new Set<string>()
  for (const branch of [...(s.oneOf ?? []), ...(s.anyOf ?? []), s.then, s.else]) {
    for (const k of branch?.required ?? []) out.add(k)
  }
  return out
}

interface ObjectFieldsProps {
  schema: JSONSchema
  value: unknown
  onChange: (v: Obj) => void
  path: string
  sensitive?: (path: string) => boolean
  /** Properties rendered elsewhere by the caller. */
  exclude?: string[]
  /** Label of the collapsible section with optional properties. */
  moreLabel?: string
}

/**
 * Renders the properties of an object schema. Required properties and the
 * ones already set come first; the others sit in a collapsible section so
 * large plugins stay readable.
 */
export function ObjectFields({
  schema: raw,
  value,
  onChange,
  path,
  sensitive,
  exclude = NO_EXCLUDE,
  moreLabel = 'Mais opções',
}: ObjectFieldsProps) {
  const schema = normalize(raw)
  const obj = isObj(value) ? value : {}
  const required = new Set(schema.required ?? [])
  const conditional = conditionallyRequired(schema)
  const keys = Object.keys(schema.properties ?? {}).filter((k) => !exclude.includes(k))
  // Decided once so fields do not jump between sections while typing.
  const [initial] = useState(
    () => new Set(keys.filter((k) => required.has(k) || conditional.has(k) || obj[k] !== undefined)),
  )
  const primary = keys.filter((k) => initial.has(k))
  const rest = keys.filter((k) => !initial.has(k))
  const [open, setOpen] = useState(primary.length === 0 && rest.length <= 4)

  const field = (k: string) => (
    <SchemaField
      key={k}
      name={k}
      schema={schema.properties?.[k] ?? {}}
      value={obj[k]}
      required={required.has(k)}
      path={path ? `${path}.${k}` : k}
      sensitive={sensitive}
      onChange={(v) => onChange(setKey(obj, k, v))}
    />
  )

  return (
    <Stack gap="sm">
      {primary.map(field)}
      {rest.length > 0 && (
        <Box>
          <UnstyledButton onClick={() => setOpen((o) => !o)}>
            <Group gap={4}>
              {open ? <IconChevronDown size={14} /> : <IconChevronRight size={14} />}
              <Text size="sm" c="dimmed">
                {moreLabel} ({rest.length})
              </Text>
            </Group>
          </UnstyledButton>
          <Collapse expanded={open}>
            <Stack gap="sm" mt="xs" pl="sm" style={{ borderLeft: '2px solid var(--mantine-color-default-border)' }}>
              {rest.map(field)}
            </Stack>
          </Collapse>
        </Box>
      )}
    </Stack>
  )
}

function KeyValueField({
  schema,
  value,
  onChange,
  name,
  required,
  path,
  sensitive,
  numeric,
}: FieldProps & { numeric: boolean }) {
  return (
    <KeyValueEditor
      label={name}
      description={describe(schema)}
      required={required}
      value={isObj(value) ? scalarRecord(value) : undefined}
      onChange={onChange}
      numeric={numeric}
      secret={sensitive?.(path)}
    />
  )
}

/** Editor for string maps such as labels or headers. */
export function KeyValueEditor({
  label,
  description,
  required,
  value,
  onChange,
  numeric,
  secret,
  keyPlaceholder = 'chave',
  valuePlaceholder = 'valor',
}: {
  label?: ReactNode
  description?: ReactNode
  required?: boolean
  value: Record<string, string | number> | undefined
  onChange: (v: Record<string, string | number> | undefined) => void
  numeric?: boolean
  secret?: boolean
  keyPlaceholder?: string
  valuePlaceholder?: string
}) {
  // Rows are kept locally so an entry with an empty key can exist while typing.
  const [rows, setRows] = useState<[string, string][]>(() =>
    Object.entries(value ?? {}).map(([k, v]) => [k, String(v)]),
  )
  const emit = (next: [string, string][]) => {
    setRows(next)
    const out: Record<string, string | number> = {}
    for (const [k, v] of next) if (k) out[k] = numeric ? Number(v) : v
    onChange(Object.keys(out).length ? out : undefined)
  }
  const ValueInput = secret ? PasswordInput : TextInput
  return (
    <Input.Wrapper label={label} description={description} withAsterisk={required}>
      <Stack gap={6} mt={4}>
        {rows.map(([k, v], i) => (
          // oxlint-disable-next-line react/no-array-index-key -- rows have no identity; inputs are controlled
          <Group key={i} gap="xs" wrap="nowrap">
            <TextInput
              placeholder={keyPlaceholder}
              value={k}
              style={{ flex: 1 }}
              onChange={(e) => emit(rows.map((r, j) => (j === i ? [e.currentTarget.value, r[1]] : r)))}
            />
            <ValueInput
              placeholder={valuePlaceholder}
              value={v}
              style={{ flex: 2 }}
              onChange={(e) => emit(rows.map((r, j) => (j === i ? [r[0], e.currentTarget.value] : r)))}
            />
            <ActionIcon variant="subtle" color="red" onClick={() => emit(rows.filter((_, j) => j !== i))}>
              <IconTrash size={16} />
            </ActionIcon>
          </Group>
        ))}
        <Button
          size="xs"
          variant="light"
          leftSection={<IconPlus size={14} />}
          onClick={() => setRows([...rows, ['', '']])}
          w="fit-content"
        >
          Adicionar
        </Button>
      </Stack>
    </Input.Wrapper>
  )
}

/** Fallback for constructs without a dedicated widget. */
export function JsonField({ schema, value, onChange, name, required }: FieldProps) {
  const [draft, setDraft] = useState(() => (value === undefined ? '' : JSON.stringify(value, null, 2)))
  const [error, setError] = useState('')
  const last = useRef(value)
  useEffect(() => {
    if (value !== last.current) {
      last.current = value
      setDraft(value === undefined ? '' : JSON.stringify(value, null, 2))
    }
  }, [value])
  return (
    <JsonInput
      label={name || undefined}
      description={[describe(schema), 'JSON'].filter(Boolean).join(' · ')}
      withAsterisk={required}
      value={draft}
      error={error}
      autosize
      minRows={2}
      maxRows={14}
      onChange={setDraft}
      onBlur={() => {
        if (!draft.trim()) {
          setError('')
          last.current = undefined
          onChange(undefined)
          return
        }
        try {
          const parsed = JSON.parse(draft)
          setError('')
          last.current = parsed
          onChange(parsed)
        } catch {
          setError('JSON inválido')
        }
      }}
      styles={{ input: { fontFamily: 'var(--mantine-font-family-monospace)', fontSize: 13 } }}
    />
  )
}
