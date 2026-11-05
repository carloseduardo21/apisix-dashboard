import { useMemo } from 'react'
import { Link } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { Anchor, Badge, Code, Group, SimpleGrid, Skeleton, Stack, Table, Tabs, Text } from '@mantine/core'
import { IconCode, IconInfoCircle, IconRoute } from '@tabler/icons-react'
import { api, itemId, type AdminItem, type AdminList } from '../api'
import type { Obj } from '../schema/types'
import { EmptyState } from './EmptyState'

const arr = (v: unknown): string[] => (Array.isArray(v) ? v.map(String) : v == null ? [] : [String(v)])

function nodes(up: Obj | undefined): string[] {
  const n = up?.nodes
  if (Array.isArray(n))
    return n.map(
      (x) =>
        `${(x as Obj).host}:${(x as Obj).port}${(x as Obj).weight !== undefined ? ` (peso ${(x as Obj).weight})` : ''}`,
    )
  if (n && typeof n === 'object') return Object.entries(n).map(([k, w]) => `${k} (peso ${w})`)
  if (up?.service_name) return [`${up.discovery_type ?? 'discovery'}: ${up.service_name}`]
  return []
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <Text size="xs" c="dimmed" tt="uppercase" fw={700}>
        {label}
      </Text>
      <Text size="sm" component="div" mt={2}>
        {children}
      </Text>
    </div>
  )
}

const none = (
  <Text size="sm" c="dimmed" span>
    —
  </Text>
)

function Upstream({ service }: { service: Obj }) {
  const id = typeof service.upstream_id === 'string' ? service.upstream_id : undefined
  const q = useQuery({
    queryKey: ['apisix', 'upstreams', id],
    queryFn: () => api<AdminItem>('GET', `/api/apisix/upstreams/${encodeURIComponent(id!)}`),
    enabled: !!id,
  })
  const up = id ? q.data?.value : (service.upstream as Obj | undefined)
  if (id && q.isPending) return <Skeleton h={60} />
  if (!up)
    return id ? (
      <Text size="sm" c="red">
        upstream {id} não encontrado
      </Text>
    ) : (
      none
    )
  return (
    <Stack gap={6}>
      <Group gap={6}>
        {id ? (
          <Anchor component={Link} to="/r/upstreams" ff="monospace" size="sm">
            {id}
          </Anchor>
        ) : (
          <Badge size="sm" variant="outline" color="gray">
            inline
          </Badge>
        )}
        <Badge size="sm" variant="light">
          {String(up.type ?? 'roundrobin')}
        </Badge>
        <Badge size="sm" variant="light" color="gray">
          {String(up.scheme ?? 'http')}
        </Badge>
        {!!up.checks && (
          <Badge size="sm" variant="light" color="teal">
            health check
          </Badge>
        )}
      </Group>
      {nodes(up).map((n) => (
        <Text key={n} size="sm" ff="monospace">
          {n}
        </Text>
      ))}
    </Stack>
  )
}

/** Service summary with the routes bound to it. */
export function ServiceDetails({ id, value }: { id: string; value: Obj }) {
  const routes = useQuery({
    queryKey: ['apisix', 'routes'],
    queryFn: () => api<AdminList>('GET', '/api/apisix/routes'),
  })
  const bound = useMemo(
    () =>
      (routes.data?.list ?? [])
        .filter((r) => r.value.service_id === id)
        .map((r) => ({ id: itemId('routes', r), v: r.value })),
    [routes.data, id],
  )
  const plugins = Object.keys((value.plugins as Obj) ?? {})
  const labels = Object.entries((value.labels as Record<string, string>) ?? {})

  return (
    <Tabs defaultValue="summary" keepMounted={false}>
      <Tabs.List mb="md">
        <Tabs.Tab value="summary" leftSection={<IconInfoCircle size={16} />}>
          Resumo
        </Tabs.Tab>
        <Tabs.Tab
          value="routes"
          leftSection={<IconRoute size={16} />}
          rightSection={
            <Badge size="xs" variant="light" circle={bound.length < 10}>
              {routes.isPending ? '…' : bound.length}
            </Badge>
          }
        >
          Routes
        </Tabs.Tab>
        <Tabs.Tab value="json" leftSection={<IconCode size={16} />}>
          JSON
        </Tabs.Tab>
      </Tabs.List>

      <Tabs.Panel value="summary">
        <Stack>
          <SimpleGrid cols={{ base: 1, sm: 2 }}>
            <Field label="Nome">{String(value.name ?? '') || none}</Field>
            <Field label="ID">
              <Code>{id}</Code>
            </Field>
          </SimpleGrid>
          <Field label="Descrição">{String(value.desc ?? '') || none}</Field>
          <Field label="Upstream">
            <Upstream service={value} />
          </Field>
          <SimpleGrid cols={{ base: 1, sm: 2 }}>
            <Field label="Hosts">{arr(value.hosts).join(', ') || none}</Field>
            <Field label="WebSocket">{value.enable_websocket ? 'habilitado' : 'desabilitado'}</Field>
          </SimpleGrid>
          <Field label="Plugins">
            {plugins.length ? (
              <Group gap={4}>
                {plugins.map((p) => (
                  <Badge key={p} size="sm" variant="light">
                    {p}
                  </Badge>
                ))}
              </Group>
            ) : (
              none
            )}
          </Field>
          <Field label="Labels">
            {labels.length ? (
              <Group gap={4}>
                {labels.map(([k, v]) => (
                  <Badge key={k} size="sm" variant="outline" color="gray">
                    {k}: {v}
                  </Badge>
                ))}
              </Group>
            ) : (
              none
            )}
          </Field>
        </Stack>
      </Tabs.Panel>

      <Tabs.Panel value="routes">
        {routes.isPending ? (
          <Skeleton h={120} />
        ) : bound.length === 0 ? (
          <EmptyState
            icon={IconRoute}
            title="Nenhuma route vinculada"
            description="Routes com service_id igual a este service aparecem aqui."
          />
        ) : (
          <Table.ScrollContainer minWidth={480}>
            <Table highlightOnHover verticalSpacing="xs" fz="sm">
              <Table.Thead>
                <Table.Tr>
                  <Table.Th>Nome</Table.Th>
                  <Table.Th>Hosts</Table.Th>
                  <Table.Th>URIs</Table.Th>
                  <Table.Th>Status</Table.Th>
                </Table.Tr>
              </Table.Thead>
              <Table.Tbody>
                {bound.map((r) => {
                  const on = r.v.status === undefined || r.v.status === 1
                  return (
                    <Table.Tr key={r.id}>
                      <Table.Td>
                        <Anchor component={Link} to={`/r/routes?editar=${encodeURIComponent(r.id)}`} size="sm" fw={500}>
                          {String(r.v.name ?? '') || r.id}
                        </Anchor>
                        <Text size="xs" c="dimmed" ff="monospace">
                          {r.id}
                        </Text>
                      </Table.Td>
                      <Table.Td ff="monospace">{arr(r.v.hosts ?? r.v.host).join(', ') || '—'}</Table.Td>
                      <Table.Td ff="monospace">{arr(r.v.uris ?? r.v.uri).join(', ') || '—'}</Table.Td>
                      <Table.Td>
                        <Badge size="sm" color={on ? 'green' : 'gray'} variant="dot">
                          {on ? 'ativo' : 'inativo'}
                        </Badge>
                      </Table.Td>
                    </Table.Tr>
                  )
                })}
              </Table.Tbody>
            </Table>
          </Table.ScrollContainer>
        )}
      </Tabs.Panel>

      <Tabs.Panel value="json">
        <Code block>{JSON.stringify(value, null, 2)}</Code>
      </Tabs.Panel>
    </Tabs>
  )
}
