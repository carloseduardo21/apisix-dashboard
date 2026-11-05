import {
  Accordion,
  ActionIcon,
  Button,
  Group,
  Input,
  NumberInput,
  SegmentedControl,
  Select,
  SimpleGrid,
  Stack,
  Text,
  TextInput,
} from '@mantine/core'
import { IconPlus, IconTrash } from '@tabler/icons-react'
import { ObjectFields, SchemaField } from '../schema/SchemaForm'
import { isObj, setKey, useCatalog, type Obj } from '../schema/types'
import { LabelsField, NameDesc, str, textProps } from './common'
import { text } from '../values'
import type { ParseKeys } from 'i18next'
import { useTranslation } from 'react-i18next'

interface Node {
  host: string
  port?: number
  weight: number
  priority?: number
}

/** Typed copy of a node object; extra fields (e.g. metadata) are preserved. */
function toNode(n: Obj): Node {
  return {
    ...n,
    host: text(n.host),
    port: typeof n.port === 'number' ? n.port : undefined,
    weight: typeof n.weight === 'number' ? n.weight : 1,
    priority: typeof n.priority === 'number' ? n.priority : undefined,
  }
}

/** APISIX accepts {"host:port": weight} or [{host, port, weight}]; the form edits the list. */
function toNodes(v: unknown): Node[] {
  if (Array.isArray(v)) return v.filter(isObj).map(toNode)
  if (isObj(v)) {
    return Object.entries(v).map(([addr, weight]) => {
      const i = addr.lastIndexOf(':')
      return i > 0
        ? { host: addr.slice(0, i), port: Number(addr.slice(i + 1)), weight: Number(weight) }
        : { host: addr, weight: Number(weight) }
    })
  }
  return []
}

function NodesEditor({ value, onChange }: { value: unknown; onChange: (v: Node[]) => void }) {
  const { t } = useTranslation()
  const nodes = toNodes(value)
  const set = (i: number, patch: Partial<Node>) => onChange(nodes.map((n, j) => (j === i ? { ...n, ...patch } : n)))
  return (
    <Input.Wrapper label={t('upstream.nodes')} withAsterisk description={t('upstream.nodes_hint')}>
      <Stack gap={6} mt={4}>
        {nodes.length > 0 && (
          <Group gap="xs" wrap="nowrap">
            <Text size="xs" c="dimmed" style={{ flex: 3 }}>
              {t('upstream.host')}
            </Text>
            <Text size="xs" c="dimmed" style={{ flex: 1 }}>
              {t('upstream.port')}
            </Text>
            <Text size="xs" c="dimmed" style={{ flex: 1 }}>
              {t('upstream.weight')}
            </Text>
            <Text size="xs" c="dimmed" style={{ flex: 1 }}>
              {t('upstream.priority')}
            </Text>
            <div style={{ width: 28 }} />
          </Group>
        )}
        {nodes.map((n, i) => (
          // oxlint-disable-next-line react/no-array-index-key -- rows have no identity and every input is controlled
          <Group key={i} gap="xs" wrap="nowrap">
            <TextInput
              style={{ flex: 3 }}
              value={n.host ?? ''}
              placeholder={t('upstream.host_placeholder')}
              onChange={(e) => set(i, { host: e.currentTarget.value })}
            />
            <NumberInput
              style={{ flex: 1 }}
              value={n.port ?? ''}
              min={1}
              max={65535}
              placeholder="80"
              onChange={(v) => set(i, { port: typeof v === 'number' ? v : undefined })}
            />
            <NumberInput
              style={{ flex: 1 }}
              value={n.weight ?? 1}
              min={0}
              onChange={(v) => set(i, { weight: typeof v === 'number' ? v : 0 })}
            />
            <NumberInput
              style={{ flex: 1 }}
              value={n.priority ?? 0}
              onChange={(v) => set(i, { priority: typeof v === 'number' && v !== 0 ? v : undefined })}
            />
            <ActionIcon variant="subtle" color="red" onClick={() => onChange(nodes.filter((_, j) => j !== i))}>
              <IconTrash size={16} />
            </ActionIcon>
          </Group>
        ))}
        <Button
          size="xs"
          variant="light"
          leftSection={<IconPlus size={14} />}
          w="fit-content"
          onClick={() => onChange([...nodes, { host: '', port: 80, weight: 1 }])}
        >
          {t('upstream.add_node')}
        </Button>
      </Stack>
    </Input.Wrapper>
  )
}

// Labels are translation keys.
const LB_TYPES = [
  { value: 'roundrobin', label: 'upstream.lb_roundrobin' },
  { value: 'chash', label: 'upstream.lb_chash' },
  { value: 'least_conn', label: 'upstream.lb_least_conn' },
  { value: 'ewma', label: 'upstream.lb_ewma' },
] as const

// [fields, title key, accordion id, hint key]
const OPTIONAL_GROUPS: [string[], ParseKeys, string, ParseKeys][] = [
  [['timeout'], 'upstream.grp_timeout', 'timeout', 'upstream.grp_timeout_hint'],
  [['retries', 'retry_timeout'], 'upstream.grp_retries', 'retries', 'upstream.grp_retries_hint'],
  [['checks'], 'upstream.grp_checks', 'checks', 'upstream.grp_checks_hint'],
  [['tls'], 'upstream.grp_tls', 'tls', 'upstream.grp_tls_hint'],
  [['keepalive_pool'], 'upstream.grp_keepalive', 'keepalive', 'upstream.grp_keepalive_hint'],
]

const SCHEMES = ['http', 'https', 'grpc', 'grpcs', 'ws', 'wss', 'tcp', 'tls', 'udp', 'kafka']

/**
 * Upstream fields. Used by the upstream resource and for inline upstreams
 * inside routes/services (`embedded` hides name/labels).
 */
export function UpstreamFields({
  value,
  onChange,
  embedded,
}: {
  value: Obj
  onChange: (v: Obj) => void
  embedded?: boolean
}) {
  const { t } = useTranslation()
  const catalog = useCatalog()
  const props = catalog.data?.resources.upstream?.properties ?? {}
  const discovery = value.service_name !== undefined || value.discovery_type !== undefined
  const type = str(value.type) || 'roundrobin'
  const sub = (key: string, label: string) =>
    props[key] && (
      <SchemaField
        name={label}
        schema={props[key]}
        value={value[key]}
        path={key}
        onChange={(v) => onChange(setKey(value, key, v))}
      />
    )

  return (
    <Stack gap="sm">
      {!embedded && <NameDesc value={value} onChange={onChange} />}

      <SimpleGrid cols={{ base: 1, sm: 2 }}>
        <Select
          label={t('upstream.balancing')}
          data={LB_TYPES.map((o) => ({ value: o.value, label: t(o.label) }))}
          value={type}
          allowDeselect={false}
          onChange={(v) =>
            onChange(setKey(setKey(setKey(value, 'type', v ?? undefined), 'hash_on', undefined), 'key', undefined))
          }
        />
        <Select
          label={t('upstream.protocol')}
          data={SCHEMES}
          value={str(value.scheme) || 'http'}
          allowDeselect={false}
          onChange={(v) => onChange(setKey(value, 'scheme', v === 'http' ? undefined : (v ?? undefined)))}
        />
      </SimpleGrid>

      {type === 'chash' && (
        <SimpleGrid cols={{ base: 1, sm: 2 }}>
          {sub('hash_on', t('upstream.hash_on'))}
          <TextInput
            label={t('upstream.hash_key')}
            withAsterisk
            description={t('upstream.hash_key_hint')}
            {...textProps(value, onChange, 'key')}
          />
        </SimpleGrid>
      )}

      <SegmentedControl
        value={discovery ? 'discovery' : 'nodes'}
        onChange={(m) =>
          onChange(
            m === 'nodes'
              ? setKey(
                  setKey(
                    setKey(setKey(value, 'service_name', undefined), 'discovery_type', undefined),
                    'discovery_args',
                    undefined,
                  ),
                  'nodes',
                  [{ host: '', port: 80, weight: 1 }],
                )
              : setKey(setKey(value, 'nodes', undefined), 'discovery_type', 'dns'),
          )
        }
        data={[
          { value: 'nodes', label: t('upstream.fixed_nodes') },
          { value: 'discovery', label: t('upstream.discovery') },
        ]}
        w="fit-content"
      />
      {discovery ? (
        <SimpleGrid cols={{ base: 1, sm: 2 }}>
          <TextInput
            label={t('upstream.discovery_type')}
            withAsterisk
            description="dns, consul, nacos, eureka, kubernetes..."
            {...textProps(value, onChange, 'discovery_type')}
          />
          <TextInput
            label={t('upstream.service_name')}
            withAsterisk
            description={t('upstream.service_name_hint')}
            {...textProps(value, onChange, 'service_name')}
          />
        </SimpleGrid>
      ) : (
        <NodesEditor value={value.nodes} onChange={(nodes) => onChange(setKey(value, 'nodes', nodes))} />
      )}

      <SimpleGrid cols={{ base: 1, sm: 2 }}>
        {sub('pass_host', t('upstream.pass_host'))}
        {value.pass_host === 'rewrite' && (
          <TextInput
            label={t('upstream.upstream_host')}
            withAsterisk
            {...textProps(value, onChange, 'upstream_host')}
          />
        )}
      </SimpleGrid>
      {/* Optional groups stay collapsed; once a group is set APISIX requires its fields. */}
      <Accordion
        multiple
        variant="contained"
        defaultValue={OPTIONAL_GROUPS.filter(([keys]) => keys.some((k) => value[k] !== undefined)).map(
          ([, , id]) => id,
        )}
      >
        {OPTIONAL_GROUPS.map(([keys, title, id, hint]) => (
          <Accordion.Item key={id} value={id}>
            <Accordion.Control>
              <Group gap="xs">
                <Text size="sm" fw={500}>
                  {t(title)}
                </Text>
                {keys.some((k) => value[k] !== undefined) && (
                  <Text size="xs" c="teal">
                    {t('upstream.configured')}
                  </Text>
                )}
              </Group>
              <Text size="xs" c="dimmed">
                {t(hint)}
              </Text>
            </Accordion.Control>
            <Accordion.Panel>
              <Stack gap="sm">
                {keys.length === 1 ? (
                  props[keys[0]] && (
                    <ObjectFields
                      schema={props[keys[0]]}
                      value={value[keys[0]]}
                      path={keys[0]}
                      onChange={(v) => onChange(setKey(value, keys[0], v))}
                      moreLabel={t('schema.options')}
                    />
                  )
                ) : (
                  <SimpleGrid cols={{ base: 1, sm: 2 }}>
                    {sub('retries', t('upstream.retries'))}
                    {sub('retry_timeout', t('upstream.retry_timeout'))}
                  </SimpleGrid>
                )}
              </Stack>
            </Accordion.Panel>
          </Accordion.Item>
        ))}
      </Accordion>
      {!embedded && <LabelsField value={value} onChange={onChange} />}
    </Stack>
  )
}

export const UPSTREAM_HANDLED = [
  'name',
  'desc',
  'type',
  'scheme',
  'hash_on',
  'key',
  'nodes',
  'service_name',
  'discovery_type',
  'discovery_args',
  'pass_host',
  'upstream_host',
  'timeout',
  'retries',
  'retry_timeout',
  'checks',
  'tls',
  'keepalive_pool',
  'labels',
]
