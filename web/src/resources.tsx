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

type Obj = Record<string, unknown>

export interface Column {
  label: string
  render: (v: Obj, id: string) => ReactNode
}

export interface ResourceDef {
  key: string
  label: string
  singular: string
  /** Grammatical gender of singular ("Nova credencial"). */
  feminine?: boolean
  description: string
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
  resource === 'ssls' && (v.labels as Obj | undefined)?.['managed-by'] === 'apisix-dashboard'

const str = (v: unknown) => (typeof v === 'string' ? v : v == null ? '' : JSON.stringify(v))
const arr = (v: unknown): unknown[] => (Array.isArray(v) ? v : v == null ? [] : [v])

function Plugins({ v }: { v: Obj }) {
  const names = Object.keys((v.plugins as Obj) ?? {})
  if (!names.length) return <Text c="dimmed" size="sm">—</Text>
  return (
    <Group gap={4}>
      {names.slice(0, 4).map((n) => (
        <Badge key={n} size="sm" variant="light">
          {n}
        </Badge>
      ))}
      {names.length > 4 && <Badge size="sm" variant="outline">+{names.length - 4}</Badge>}
    </Group>
  )
}

function Status({ v }: { v: Obj }) {
  const on = v.status === undefined || v.status === 1
  return (
    <Badge size="sm" color={on ? 'green' : 'gray'} variant="dot">
      {on ? 'ativo' : 'inativo'}
    </Badge>
  )
}

function nodesText(nodes: unknown): string {
  if (Array.isArray(nodes)) {
    return nodes.map((n) => `${(n as Obj).host}:${(n as Obj).port}`).join(', ')
  }
  if (nodes && typeof nodes === 'object') return Object.keys(nodes).join(', ')
  return ''
}

function upstreamText(v: Obj): string {
  if (v.upstream_id) return `upstream ${v.upstream_id}`
  if (v.service_id) return `service ${v.service_id}`
  const up = v.upstream as Obj | undefined
  if (up) return up.service_name ? `discovery: ${up.service_name}` : nodesText(up.nodes)
  return ''
}

const mono = (s: string) => (
  <Text size="sm" ff="monospace" lineClamp={1}>
    {s || '—'}
  </Text>
)

const nameCol: Column = { label: 'Nome', render: (v) => <Text size="sm" fw={500}>{str(v.name) || '—'}</Text> }
const idCol: Column = { label: 'ID', render: (_v, id) => mono(id) }
const descCol: Column = { label: 'Descrição', render: (v) => <Text size="sm" c="dimmed" lineClamp={1}>{str(v.desc) || '—'}</Text> }
const pluginsCol: Column = { label: 'Plugins', render: (v) => <Plugins v={v} /> }

export const RESOURCES: ResourceDef[] = [
  {
    key: 'routes',
    description: 'Regras que associam requisições a upstreams e plugins',
    label: 'Routes',
    singular: 'route',
    icon: IconRoute,
    create: 'post',
    columns: [
      idCol,
      nameCol,
      { label: 'Hosts', render: (v) => mono(arr(v.hosts ?? v.host).join(', ')) },
      { label: 'URIs', render: (v) => mono(arr(v.uris ?? v.uri).join(', ')) },
      { label: 'Status', render: (v) => <Status v={v} /> },
    ],
    template: { name: '' },
    search: (v, id) => [id, v.name, v.uri, v.uris, v.host, v.hosts, v.desc].map(str).join(' '),
  },
  {
    key: 'services',
    description: 'Agrupam configuração compartilhada entre routes',
    label: 'Services',
    singular: 'service',
    icon: IconStack2,
    create: 'post',
    columns: [idCol, nameCol, descCol, { label: 'Destino', render: (v) => mono(upstreamText(v)) }, pluginsCol],
    template: { name: '' },
    search: (v, id) => [id, v.name, v.desc].map(str).join(' '),
  },
  {
    key: 'upstreams',
    description: 'Destinos de backend e balanceamento de carga',
    label: 'Upstreams',
    singular: 'upstream',
    icon: IconServer,
    create: 'post',
    columns: [
      idCol,
      nameCol,
      { label: 'Balanceamento', render: (v) => <Badge size="sm" variant="light">{str(v.type) || 'roundrobin'}</Badge> },
      { label: 'Nós', render: (v) => mono(v.service_name ? `${v.discovery_type ?? 'discovery'}: ${v.service_name}` : nodesText(v.nodes)) },
      { label: 'Esquema', render: (v) => <Text size="sm">{str(v.scheme) || 'http'}</Text> },
      { label: 'Health check', render: (v) => (v.checks ? <Badge size="sm" color="teal" variant="light">sim</Badge> : <Text size="sm" c="dimmed">—</Text>) },
    ],
    template: { name: '', type: 'roundrobin', nodes: [{ host: '', port: 80, weight: 1 }] },
    search: (v, id) => [id, v.name, v.desc, nodesText(v.nodes), v.service_name].map(str).join(' '),
  },
  {
    key: 'consumers',
    description: 'Clientes da API e suas credenciais',
    label: 'Consumers',
    singular: 'consumer',
    icon: IconUsers,
    create: 'put-body',
    columns: [
      { label: 'Usuário', render: (v) => <Text size="sm" fw={500}>{str(v.username)}</Text> },
      descCol,
      { label: 'Grupo', render: (v) => mono(str(v.group_id)) },
      pluginsCol,
    ],
    template: { username: '' },
    search: (v, id) => [id, v.username, v.desc, v.group_id].map(str).join(' '),
  },
  {
    key: 'consumer_groups',
    description: 'Plugins compartilhados entre consumers',
    label: 'Consumer Groups',
    singular: 'consumer group',
    icon: IconUsersGroup,
    create: 'put-id',
    columns: [idCol, descCol, pluginsCol],
    template: { plugins: {} },
    search: (v, id) => [id, v.desc].map(str).join(' '),
  },
  {
    key: 'ssls',
    description: 'Certificados TLS servidos pelo gateway',
    label: 'Certificados SSL',
    singular: 'certificado',
    icon: IconCertificate,
    create: 'post',
    columns: [
      idCol,
      { label: 'SNIs', render: (v) => mono(arr(v.snis ?? v.sni).join(', ')) },
      { label: 'Tipo', render: (v) => <Text size="sm">{str(v.type) || 'server'}</Text> },
      {
        label: 'Origem',
        render: (v) =>
          isManagedSSL('ssls', v) ? (
            <Badge size="sm" variant="light" color="orange">
              automático
            </Badge>
          ) : (
            <Text size="sm" c="dimmed">manual</Text>
          ),
      },
      {
        label: 'Validade',
        render: (v) => {
          const end = typeof v.validity_end === 'number' ? new Date(v.validity_end * 1000) : null
          if (!end) return <Text size="sm" c="dimmed">—</Text>
          const days = Math.floor((end.getTime() - Date.now()) / 86_400_000)
          const color = days < 0 ? 'red' : days < 30 ? 'orange' : 'green'
          return (
            <Badge size="sm" color={color} variant="light">
              {end.toLocaleDateString('pt-BR')} ({days}d)
            </Badge>
          )
        },
      },
      { label: 'Status', render: (v) => <Status v={v} /> },
    ],
    template: {},
    search: (v, id) => [id, v.snis, v.sni].map(str).join(' '),
  },
  {
    key: 'plugin_configs',
    description: 'Conjuntos reutilizáveis de plugins',
    label: 'Plugin Configs',
    singular: 'plugin config',
    icon: IconPlug,
    create: 'put-id',
    columns: [idCol, descCol, pluginsCol],
    template: { plugins: {} },
    search: (v, id) => [id, v.desc].map(str).join(' '),
  },
  {
    key: 'global_rules',
    description: 'Plugins executados em todas as requisições',
    label: 'Global Rules',
    singular: 'global rule',
    icon: IconWorld,
    create: 'put-id',
    columns: [idCol, pluginsCol],
    template: { plugins: {} },
    search: (_v, id) => id,
  },
  {
    key: 'stream_routes',
    description: 'Proxy TCP/UDP (camada 4)',
    label: 'Stream Routes',
    singular: 'stream route',
    icon: IconArrowsSplit,
    create: 'post',
    columns: [
      idCol,
      { label: 'Porta', render: (v) => mono(str(v.server_port)) },
      { label: 'Origem', render: (v) => mono(str(v.remote_addr)) },
      { label: 'SNI', render: (v) => mono(str(v.sni)) },
      { label: 'Destino', render: (v) => mono(upstreamText(v)) },
    ],
    template: {},
    search: (v, id) => [id, v.server_port, v.remote_addr, v.sni].map(str).join(' '),
  },
  {
    key: 'protos',
    description: 'Definições Protobuf para gRPC',
    label: 'Protos',
    singular: 'proto',
    icon: IconFileCode,
    create: 'put-id',
    columns: [idCol, descCol],
    template: { content: 'syntax = "proto3";\n' },
    search: (v, id) => [id, v.desc].map(str).join(' '),
  },
  {
    key: 'secrets',
    description: 'Integração com gerenciadores de segredos',
    label: 'Secrets',
    singular: 'secret manager',
    icon: IconLock,
    create: 'put-id',
    columns: [idCol, { label: 'URI', render: (v) => mono(str(v.uri)) }, { label: 'Prefixo', render: (v) => mono(str(v.prefix)) }],
    template: {},
    search: (v, id) => [id, v.uri, v.prefix].map(str).join(' '),
  },
]

export const RESOURCE_BY_KEY = Object.fromEntries(RESOURCES.map((r) => [r.key, r]))

export const MAIN_RESOURCES = ['routes', 'services', 'upstreams', 'consumers', 'ssls', 'plugin_configs', 'global_rules', 'stream_routes']

