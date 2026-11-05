import type { ReactNode } from 'react'
import { Badge, Group, Text } from '@mantine/core'
import {
  IconArrowsSplit,
  IconCertificate,
  IconPlug,
  IconRoute,
  IconServer,
  IconStack2,
  IconUsers,
  IconUsersGroup,
  IconWorld,
  IconFileCode,
  IconLock,
  type Icon,
} from '@tabler/icons-react'
import type { ParseKeys } from 'i18next'
import { useTranslation } from 'react-i18next'
import i18n from './i18n'
import { formatDate } from './i18n/format'
import { asObj, text, type Obj } from './values'

export interface Column {
  /** Translation key of the header. */
  label: ParseKeys
  render: (v: Obj, id: string) => ReactNode
}

export type ResourceKey =
  | 'routes'
  | 'services'
  | 'upstreams'
  | 'consumers'
  | 'consumer_groups'
  | 'ssls'
  | 'plugin_configs'
  | 'global_rules'
  | 'stream_routes'
  | 'protos'
  | 'secrets'
  | 'credentials'

/** An APISIX resource. Its label, singular, gender and description come from
 * the catalog: resources.<key>.* */
export interface ResourceDef {
  key: ResourceKey
  icon: Icon
  columns: Column[]
  /** How new objects are created. */
  create: 'post' | 'put-id' | 'put-body'
  template: Obj
  /** Text searched by the list filter. */
  search: (v: Obj, id: string) => string
}

/** SSL objects issued by the dashboard's ACME module: edited only there. */
export const isManagedSSL = (resource: string, v: Obj) =>
  resource === 'ssls' && asObj(v.labels)?.['managed-by'] === 'apisix-dashboard'

const str = text
const arr = (v: unknown): unknown[] => (Array.isArray(v) ? v : v == null ? [] : [v])

function Plugins({ v }: { v: Obj }) {
  const names = Object.keys(asObj(v.plugins) ?? {})
  if (!names.length)
    return (
      <Text c="dimmed" size="sm">
        —
      </Text>
    )
  return (
    <Group gap={4}>
      {names.slice(0, 4).map((n) => (
        <Badge key={n} size="sm" variant="light">
          {n}
        </Badge>
      ))}
      {names.length > 4 && (
        <Badge size="sm" variant="outline">
          +{names.length - 4}
        </Badge>
      )}
    </Group>
  )
}

function Status({ v }: { v: Obj }) {
  const { t } = useTranslation()
  const on = v.status === undefined || v.status === 1
  return (
    <Badge size="sm" color={on ? 'green' : 'gray'} variant="dot">
      {on ? t('common.active') : t('common.inactive')}
    </Badge>
  )
}

function nodesText(nodes: unknown): string {
  if (Array.isArray(nodes)) {
    return nodes
      .map((raw) => {
        const n = asObj(raw) ?? {}
        return `${text(n.host)}:${text(n.port)}`
      })
      .join(', ')
  }
  if (nodes && typeof nodes === 'object') return Object.keys(nodes).join(', ')
  return ''
}

function upstreamText(v: Obj): string {
  if (v.upstream_id) return `upstream ${text(v.upstream_id)}`
  if (v.service_id) return `service ${text(v.service_id)}`
  const up = asObj(v.upstream)
  if (up) return up.service_name ? `discovery: ${text(up.service_name)}` : nodesText(up.nodes)
  return ''
}

const mono = (s: string) => (
  <Text size="sm" ff="monospace" lineClamp={1}>
    {s || '—'}
  </Text>
)

const nameCol: Column = {
  label: 'columns.name',
  render: (v) => (
    <Text size="sm" fw={500}>
      {str(v.name) || '—'}
    </Text>
  ),
}
const idCol: Column = { label: 'columns.id', render: (_v, id) => mono(id) }
const descCol: Column = {
  label: 'columns.description',
  render: (v) => (
    <Text size="sm" c="dimmed" lineClamp={1}>
      {str(v.desc) || '—'}
    </Text>
  ),
}
const pluginsCol: Column = { label: 'columns.plugins', render: (v) => <Plugins v={v} /> }

export const RESOURCES: ResourceDef[] = [
  {
    key: 'routes',
    icon: IconRoute,
    create: 'post',
    columns: [
      idCol,
      nameCol,
      { label: 'columns.hosts', render: (v) => mono(arr(v.hosts ?? v.host).join(', ')) },
      { label: 'columns.uris', render: (v) => mono(arr(v.uris ?? v.uri).join(', ')) },
      { label: 'columns.status', render: (v) => <Status v={v} /> },
    ],
    template: { name: '' },
    search: (v, id) => [id, v.name, v.uri, v.uris, v.host, v.hosts, v.desc].map(str).join(' '),
  },
  {
    key: 'services',
    icon: IconStack2,
    create: 'post',
    columns: [idCol, nameCol, descCol, { label: 'columns.target', render: (v) => mono(upstreamText(v)) }, pluginsCol],
    template: { name: '' },
    search: (v, id) => [id, v.name, v.desc].map(str).join(' '),
  },
  {
    key: 'upstreams',
    icon: IconServer,
    create: 'post',
    columns: [
      idCol,
      nameCol,
      {
        label: 'columns.balancing',
        render: (v) => (
          <Badge size="sm" variant="light">
            {str(v.type) || 'roundrobin'}
          </Badge>
        ),
      },
      {
        label: 'columns.nodes',
        render: (v) =>
          mono(
            v.service_name ? `${text(v.discovery_type ?? 'discovery')}: ${text(v.service_name)}` : nodesText(v.nodes),
          ),
      },
      { label: 'columns.scheme', render: (v) => <Text size="sm">{str(v.scheme) || 'http'}</Text> },
      {
        label: 'columns.health_check',
        render: (v) =>
          v.checks ? (
            <Badge size="sm" color="teal" variant="light">
              {i18n.t('common.yes')}
            </Badge>
          ) : (
            <Text size="sm" c="dimmed">
              —
            </Text>
          ),
      },
    ],
    template: { name: '', type: 'roundrobin', nodes: [{ host: '', port: 80, weight: 1 }] },
    search: (v, id) => [id, v.name, v.desc, nodesText(v.nodes), v.service_name].map(str).join(' '),
  },
  {
    key: 'consumers',
    icon: IconUsers,
    create: 'put-body',
    columns: [
      {
        label: 'columns.user',
        render: (v) => (
          <Text size="sm" fw={500}>
            {str(v.username)}
          </Text>
        ),
      },
      descCol,
      { label: 'columns.group', render: (v) => mono(str(v.group_id)) },
      pluginsCol,
    ],
    template: { username: '' },
    search: (v, id) => [id, v.username, v.desc, v.group_id].map(str).join(' '),
  },
  {
    key: 'consumer_groups',
    icon: IconUsersGroup,
    create: 'put-id',
    columns: [idCol, descCol, pluginsCol],
    template: { plugins: {} },
    search: (v, id) => [id, v.desc].map(str).join(' '),
  },
  {
    key: 'ssls',
    icon: IconCertificate,
    create: 'post',
    columns: [
      idCol,
      { label: 'columns.snis', render: (v) => mono(arr(v.snis ?? v.sni).join(', ')) },
      { label: 'columns.type', render: (v) => <Text size="sm">{str(v.type) || 'server'}</Text> },
      {
        label: 'columns.origin',
        render: (v) =>
          isManagedSSL('ssls', v) ? (
            <Badge size="sm" variant="light" color="orange">
              {i18n.t('columns.automatic')}
            </Badge>
          ) : (
            <Text size="sm" c="dimmed">
              {i18n.t('columns.manual')}
            </Text>
          ),
      },
      {
        label: 'columns.validity',
        render: (v) => {
          const end = typeof v.validity_end === 'number' ? new Date(v.validity_end * 1000) : null
          if (!end)
            return (
              <Text size="sm" c="dimmed">
                —
              </Text>
            )
          const days = Math.floor((end.getTime() - Date.now()) / 86_400_000)
          const color = days < 0 ? 'red' : days < 30 ? 'orange' : 'green'
          return (
            <Badge size="sm" color={color} variant="light">
              {formatDate(end)} ({days}d)
            </Badge>
          )
        },
      },
      { label: 'columns.status', render: (v) => <Status v={v} /> },
    ],
    template: {},
    search: (v, id) => [id, v.snis, v.sni].map(str).join(' '),
  },
  {
    key: 'plugin_configs',
    icon: IconPlug,
    create: 'put-id',
    columns: [idCol, descCol, pluginsCol],
    template: { plugins: {} },
    search: (v, id) => [id, v.desc].map(str).join(' '),
  },
  {
    key: 'global_rules',
    icon: IconWorld,
    create: 'put-id',
    columns: [idCol, pluginsCol],
    template: { plugins: {} },
    search: (_v, id) => id,
  },
  {
    key: 'stream_routes',
    icon: IconArrowsSplit,
    create: 'post',
    columns: [
      idCol,
      { label: 'columns.port', render: (v) => mono(str(v.server_port)) },
      { label: 'columns.source', render: (v) => mono(str(v.remote_addr)) },
      { label: 'columns.sni', render: (v) => mono(str(v.sni)) },
      { label: 'columns.target', render: (v) => mono(upstreamText(v)) },
    ],
    template: {},
    search: (v, id) => [id, v.server_port, v.remote_addr, v.sni].map(str).join(' '),
  },
  {
    key: 'protos',
    icon: IconFileCode,
    create: 'put-id',
    columns: [idCol, descCol],
    template: { content: 'syntax = "proto3";\n' },
    search: (v, id) => [id, v.desc].map(str).join(' '),
  },
  {
    key: 'secrets',
    icon: IconLock,
    create: 'put-id',
    columns: [
      idCol,
      { label: 'columns.uri', render: (v) => mono(str(v.uri)) },
      { label: 'columns.prefix', render: (v) => mono(str(v.prefix)) },
    ],
    template: {},
    search: (v, id) => [id, v.uri, v.prefix].map(str).join(' '),
  },
]

export const RESOURCE_BY_KEY: Partial<Record<string, ResourceDef>> = Object.fromEntries(
  RESOURCES.map((r) => [r.key, r]),
)

export const MAIN_RESOURCES: ResourceKey[] = [
  'routes',
  'services',
  'upstreams',
  'consumers',
  'ssls',
  'plugin_configs',
  'global_rules',
  'stream_routes',
]
