import { useMemo, useState } from 'react'
import {
  Accordion,
  ActionIcon,
  Alert,
  Anchor,
  Badge,
  Group,
  Loader,
  SegmentedControl,
  Select,
  Stack,
  Switch,
  Text,
  Tooltip,
} from '@mantine/core'
import { IconAlertCircle, IconExternalLink, IconPlug, IconTrash } from '@tabler/icons-react'
import { JsonField, ObjectFields } from './SchemaForm'
import { isObj, sensitiveMatcher, setKey, useCatalog, type JSONSchema, type Obj, type PluginInfo } from './types'
import type { ParseKeys } from 'i18next'
import { useTranslation } from 'react-i18next'

/** Plugin groups in the add menu: [translation key, test]. */
const CATEGORIES: [ParseKeys, (name: string, info: PluginInfo) => boolean][] = [
  [
    'schema.cat.auth',
    (n, i) =>
      i.type === 'auth' ||
      n.endsWith('-auth') ||
      ['openid-connect', 'authz-keycloak', 'authz-casbin', 'authz-casdoor', 'wolf-rbac', 'opa', 'jwe-decrypt'].includes(
        n,
      ),
  ],
  [
    'schema.cat.security',
    (n) =>
      [
        'cors',
        'csrf',
        'uri-blocker',
        'consumer-restriction',
        'chaitin-waf',
        'request-validation',
        'public-api',
        'gm',
      ].includes(n) || n.endsWith('-restriction'),
  ],
  [
    'schema.cat.traffic',
    (n) =>
      n.startsWith('limit-') ||
      n.startsWith('traffic-') ||
      [
        'api-breaker',
        'proxy-cache',
        'proxy-mirror',
        'request-id',
        'client-control',
        'workflow',
        'real-ip',
        'proxy-control',
        'proxy-buffering',
        'graphql-limit-count',
      ].includes(n),
  ],
  [
    'schema.cat.transform',
    (n) =>
      n.includes('rewrite') ||
      n.includes('transform') ||
      n.includes('transcode') ||
      [
        'fault-injection',
        'mocking',
        'redirect',
        'echo',
        'degraphql',
        'gzip',
        'brotli',
        'attach-consumer-label',
        'inspect',
      ].includes(n),
  ],
  [
    'schema.cat.observability',
    (n) =>
      n.endsWith('-logger') ||
      n.endsWith('-logging') ||
      [
        'prometheus',
        'zipkin',
        'skywalking',
        'opentelemetry',
        'datadog',
        'node-status',
        'server-info',
        'loggly',
        'lago',
      ].includes(n),
  ],
  ['schema.cat.ai', (n) => n.startsWith('ai')],
  [
    'schema.cat.serverless',
    (n) =>
      n.startsWith('serverless') ||
      n.startsWith('ext-plugin') ||
      n.includes('function') ||
      ['aws-lambda', 'openwhisk', 'openfunction'].includes(n),
  ],
]

function category(name: string, info: PluginInfo): ParseKeys {
  return CATEGORIES.find(([, test]) => test(name, info))?.[0] ?? 'schema.cat.other'
}

interface Props {
  value: Obj | undefined
  onChange: (v: Obj | undefined) => void
  /** Consumers use each auth plugin's consumer_schema. */
  consumer?: boolean
  /** Only authentication plugins (consumer credentials). */
  authOnly?: boolean
  subsystem?: 'http' | 'stream'
}

const NO_PLUGINS: Obj = {}

export function PluginsEditor({ value, onChange, consumer, authOnly, subsystem = 'http' }: Props) {
  const { t } = useTranslation()
  const catalog = useCatalog()
  const plugins = isObj(value) ? value : NO_PLUGINS
  const enabled = Object.keys(plugins)
  const [open, setOpen] = useState<string[]>([])
  const [modes, setModes] = useState<Record<string, 'form' | 'json'>>({})
  const [search, setSearch] = useState('')

  const available = subsystem === 'stream' ? catalog.data?.stream_plugins : catalog.data?.plugins
  // Grouped by category key; labels are translated at render time.
  const grouped = useMemo(() => {
    const groups: Partial<Record<ParseKeys, { value: string; label: string }[]>> = {}
    for (const [name, info] of Object.entries(available ?? {}).toSorted(([a], [b]) => a.localeCompare(b))) {
      if (name in plugins || (authOnly && info.type !== 'auth')) continue
      ;(groups[category(name, info)] ??= []).push({ value: name, label: name })
    }
    return groups
  }, [available, plugins, authOnly])
  const options = CATEGORIES.map(([key]) => key)
    .concat('schema.cat.other')
    .filter((key) => grouped[key])
    .map((key) => ({ group: t(key), items: grouped[key] ?? [] }))

  if (catalog.isPending) return <Loader size="sm" />
  if (catalog.error) {
    return (
      <Alert color="red" icon={<IconAlertCircle size={16} />}>
        {t('schema.plugins_load_error', { error: catalog.error.message })}
      </Alert>
    )
  }

  const update = (name: string, conf: Obj | undefined) => {
    const next = { ...plugins }
    if (conf === undefined) delete next[name]
    else next[name] = conf
    onChange(Object.keys(next).length ? next : undefined)
  }

  const add = (name: string | null) => {
    if (!name) return
    update(name, {})
    setOpen((o) => [...o, name])
    setSearch('')
  }

  return (
    <Stack gap="sm">
      <Group justify="space-between">
        <Text size="sm" c="dimmed">
          {enabled.length ? t('schema.plugins_count', { count: enabled.length }) : t('schema.no_plugins')}
        </Text>
        <Select
          placeholder={t('schema.add_plugin')}
          leftSection={<IconPlug size={16} />}
          data={options}
          value={null}
          onChange={add}
          searchValue={search}
          onSearchChange={setSearch}
          searchable
          nothingFoundMessage={t('schema.no_plugin_found')}
          w={300}
          maxDropdownHeight={360}
          comboboxProps={{ withinPortal: true }}
        />
      </Group>

      {enabled.length > 0 && (
        <Accordion multiple value={open} onChange={setOpen} variant="separated" chevronPosition="left">
          {enabled.map((name) => {
            const info = available?.[name]
            const conf = isObj(plugins[name]) ? plugins[name] : {}
            const meta = isObj(conf._meta) ? conf._meta : {}
            const disabled = meta.disable === true
            const schema: JSONSchema | undefined =
              info && (consumer && info.consumer_schema ? info.consumer_schema : info.schema)
            const mode = modes[name] ?? (schema ? 'form' : 'json')
            const sensitive = sensitiveMatcher(
              schema?.encrypt_fields,
              consumer ? [] : info?.consumer_schema?.encrypt_fields,
            )

            return (
              <Accordion.Item key={name} value={name}>
                <Accordion.Control>
                  <Group gap="xs">
                    <Text fw={500} ff="monospace" size="sm">
                      {name}
                    </Text>
                    {disabled && (
                      <Badge size="xs" color="gray">
                        desativado
                      </Badge>
                    )}
                    {!info && (
                      <Badge size="xs" color="orange">
                        desconhecido neste APISIX
                      </Badge>
                    )}
                  </Group>
                </Accordion.Control>
                <Accordion.Panel>
                  <Stack gap="sm">
                    <Group justify="space-between">
                      <Group gap="md">
                        <Switch
                          size="xs"
                          label={t('schema.plugin_active')}
                          checked={!disabled}
                          onChange={(e) => {
                            const nextMeta = setKey(meta, 'disable', e.currentTarget.checked ? undefined : true)
                            update(name, setKey(conf, '_meta', nextMeta))
                          }}
                        />
                        <SegmentedControl
                          size="xs"
                          value={mode}
                          onChange={(m) => setModes((s) => ({ ...s, [name]: m }))}
                          data={[
                            { value: 'form', label: t('editor.form_tab'), disabled: !schema },
                            { value: 'json', label: 'JSON' },
                          ]}
                        />
                      </Group>
                      <Group gap="xs">
                        <Anchor
                          href={`https://apisix.apache.org/docs/apisix/plugins/${name}/`}
                          target="_blank"
                          rel="noreferrer"
                          size="xs"
                        >
                          <Group gap={2}>
                            {t('schema.docs')} <IconExternalLink size={12} />
                          </Group>
                        </Anchor>
                        <Tooltip label={t('schema.remove_plugin')}>
                          <ActionIcon variant="subtle" color="red" onClick={() => update(name, undefined)}>
                            <IconTrash size={16} />
                          </ActionIcon>
                        </Tooltip>
                      </Group>
                    </Group>
                    {mode === 'form' && schema ? (
                      <ObjectFields
                        schema={schema}
                        value={conf}
                        onChange={(v) => update(name, v)}
                        path=""
                        sensitive={sensitive}
                        moreLabel={t('schema.options')}
                      />
                    ) : (
                      <JsonField
                        name=""
                        schema={{}}
                        path=""
                        value={conf}
                        onChange={(v) => update(name, isObj(v) ? v : {})}
                      />
                    )}
                  </Stack>
                </Accordion.Panel>
              </Accordion.Item>
            )
          })}
        </Accordion>
      )}
    </Stack>
  )
}
