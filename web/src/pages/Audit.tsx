import { useMemo, useState } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import {
  Avatar,
  Badge,
  Button,
  Card,
  Code,
  Group,
  Modal,
  ScrollArea,
  SegmentedControl,
  SimpleGrid,
  Stack,
  Table,
  Text,
  TextInput,
} from '@mantine/core'
import { IconHistory, IconRefresh, IconSearch } from '@tabler/icons-react'
import { api, type AuditEvent } from '../api'
import { EmptyState } from '../components/EmptyState'
import { PageHeader } from '../components/PageHeader'
import { avatarColor, initials } from '../components/roles'

interface Page {
  items: AuditEvent[]
  next_before?: number
}

const LABELS: Record<string, string> = {
  'auth.login': 'login',
  'auth.logout': 'logout',
  'auth.password': 'senha validada',
  'auth.mfa': 'MFA',
  'auth.mfa_enroll': 'MFA configurado',
  'auth.lockout': 'conta bloqueada',
  'auth.password_change': 'troca de senha',
  'apisix.create': 'criou',
  'apisix.put': 'salvou',
  'apisix.patch': 'alterou',
  'apisix.delete': 'excluiu',
  'user.create': 'criou usuário',
  'user.update': 'alterou usuário',
  'user.delete': 'excluiu usuário',
  'user.password_reset': 'redefiniu senha',
  'user.mfa_reset': 'redefiniu MFA',
  'user.unlock': 'desbloqueou',
  'acme.create': 'cadastrou certificado',
  'acme.issue': 'emitiu certificado',
  'acme.delete': 'removeu certificado',
  'kong.migrate': 'migrou do Kong',
  'kong.rollback': 'desfez migração',
  'kong.migration': 'lote de migração',
  'kong.settings': 'padrões da migração',
}

export const actionLabel = (a: string) => LABELS[a] ?? a

export function actionColor(a: string) {
  if (a === 'apisix.delete' || a === 'user.delete' || a === 'auth.lockout') return 'red'
  if (a === 'acme.delete') return 'red'
  if (a.startsWith('acme.')) return 'orange'
  if (a === 'kong.rollback') return 'orange'
  if (a.startsWith('kong.')) return 'cyan'
  if (a.startsWith('apisix.')) return 'indigo'
  if (a.startsWith('user.')) return 'grape'
  return 'gray'
}

const json = (v: unknown) => (v === undefined || v === null ? '—' : JSON.stringify(v, null, 2))

type Filter = 'all' | 'changes' | 'auth' | 'failures'

const FILTERS: Record<Filter, (e: AuditEvent) => boolean> = {
  all: () => true,
  changes: (e) => e.action.startsWith('apisix.') || e.action.startsWith('user.') || e.action.startsWith('acme.'),
  auth: (e) => e.action.startsWith('auth.'),
  failures: (e) => e.outcome === 'failure',
}

export function Audit() {
  const [selected, setSelected] = useState<AuditEvent | null>(null)
  const [filter, setFilter] = useState<Filter>('all')
  const [search, setSearch] = useState('')
  const q = useInfiniteQuery({
    queryKey: ['audit'],
    queryFn: ({ pageParam }) => api<Page>('GET', `/api/audit?limit=100${pageParam ? `&before=${pageParam}` : ''}`),
    initialPageParam: 0,
    getNextPageParam: (last) => last.next_before,
  })

  const events = useMemo(() => {
    const all = q.data?.pages.flatMap((p) => p.items) ?? []
    const s = search.trim().toLowerCase()
    return all
      .filter(FILTERS[filter])
      .filter(
        (e) => !s || [e.actor, e.ip, e.action, e.resource, e.resource_id, e.detail].join(' ').toLowerCase().includes(s),
      )
  }, [q.data, filter, search])

  return (
    <>
      <PageHeader
        icon={IconHistory}
        title="Auditoria"
        description="Autenticação, gestão de usuários e alterações no APISIX. Segredos são mascarados."
        actions={
          <Button
            variant="default"
            leftSection={<IconRefresh size={16} />}
            onClick={() => q.refetch()}
            loading={q.isFetching && !q.isFetchingNextPage}
          >
            Atualizar
          </Button>
        }
      />

      <Card padding={0}>
        <Group justify="space-between" p="md" gap="sm">
          <SegmentedControl
            value={filter}
            onChange={(v) => setFilter(v)}
            data={[
              { value: 'all', label: 'Todos' },
              { value: 'changes', label: 'Alterações' },
              { value: 'auth', label: 'Acessos' },
              { value: 'failures', label: 'Falhas' },
            ]}
          />
          <TextInput
            placeholder="Buscar usuário, IP, recurso..."
            leftSection={<IconSearch size={16} />}
            value={search}
            onChange={(e) => setSearch(e.currentTarget.value)}
            w={300}
          />
        </Group>
        {events.length === 0 && !q.isPending ? (
          <EmptyState
            icon={IconHistory}
            title="Nenhum evento"
            description="Nenhum evento corresponde ao filtro selecionado."
          />
        ) : (
          <Table.ScrollContainer minWidth={900}>
            <Table highlightOnHover verticalSpacing="sm" horizontalSpacing="md">
              <Table.Thead bg="var(--mantine-color-default-hover)">
                <Table.Tr>
                  <Table.Th>Quando</Table.Th>
                  <Table.Th>Usuário</Table.Th>
                  <Table.Th>Ação</Table.Th>
                  <Table.Th>Recurso</Table.Th>
                  <Table.Th>IP</Table.Th>
                  <Table.Th>Detalhe</Table.Th>
                </Table.Tr>
              </Table.Thead>
              <Table.Tbody>
                {events.map((e) => (
                  <Table.Tr key={e.id} style={{ cursor: 'pointer' }} onClick={() => setSelected(e)}>
                    <Table.Td style={{ whiteSpace: 'nowrap' }}>
                      <Text size="sm">{new Date(e.ts).toLocaleDateString('pt-BR')}</Text>
                      <Text size="xs" c="dimmed">
                        {new Date(e.ts).toLocaleTimeString('pt-BR')}
                      </Text>
                    </Table.Td>
                    <Table.Td>
                      <Group gap="xs" wrap="nowrap">
                        <Avatar size={28} radius="xl" color={avatarColor(e.actor)}>
                          {initials(e.actor.replace(/^cli:/, ''))}
                        </Avatar>
                        <Text size="sm" fw={500}>
                          {e.actor}
                        </Text>
                      </Group>
                    </Table.Td>
                    <Table.Td style={{ whiteSpace: 'nowrap' }}>
                      <Badge variant="light" color={e.outcome === 'failure' ? 'red' : actionColor(e.action)}>
                        {e.outcome === 'failure' ? `${actionLabel(e.action)} · falhou` : actionLabel(e.action)}
                      </Badge>
                    </Table.Td>
                    <Table.Td>
                      {e.resource ? (
                        <Text size="sm" ff="monospace">
                          {e.resource}
                          {e.resource_id && (
                            <Text span c="dimmed" ff="monospace">
                              /{e.resource_id}
                            </Text>
                          )}
                        </Text>
                      ) : (
                        <Text size="sm" c="dimmed">
                          —
                        </Text>
                      )}
                    </Table.Td>
                    <Table.Td>
                      <Text size="sm" ff="monospace" c="dimmed">
                        {e.ip || '—'}
                      </Text>
                    </Table.Td>
                    <Table.Td>
                      <Text size="sm" c="dimmed" lineClamp={1} maw={300}>
                        {e.detail || '—'}
                      </Text>
                    </Table.Td>
                  </Table.Tr>
                ))}
              </Table.Tbody>
            </Table>
          </Table.ScrollContainer>
        )}
        {q.hasNextPage && (
          <Group justify="center" p="md">
            <Button variant="default" onClick={() => q.fetchNextPage()} loading={q.isFetchingNextPage}>
              Carregar mais
            </Button>
          </Group>
        )}
      </Card>

      <Modal
        opened={!!selected}
        onClose={() => setSelected(null)}
        size="90%"
        title={<Text fw={600}>Evento #{selected?.id}</Text>}
      >
        {selected && (
          <Stack>
            <Group gap="sm">
              <Avatar radius="xl" color={avatarColor(selected.actor)}>
                {initials(selected.actor.replace(/^cli:/, ''))}
              </Avatar>
              <div>
                <Text fw={600}>
                  {selected.actor}{' '}
                  <Badge variant="light" color={selected.outcome === 'failure' ? 'red' : actionColor(selected.action)}>
                    {actionLabel(selected.action)}
                  </Badge>
                </Text>
                <Text size="sm" c="dimmed">
                  {new Date(selected.ts).toLocaleString('pt-BR')} · {selected.ip || 'sem IP'}
                  {selected.resource && ` · ${selected.resource}/${selected.resource_id ?? ''}`}
                </Text>
              </div>
            </Group>
            {selected.detail && <Text size="sm">{selected.detail}</Text>}
            {(selected.before !== undefined || selected.after !== undefined) && (
              <SimpleGrid cols={{ base: 1, md: 2 }}>
                {(['before', 'after'] as const).map((k) => (
                  <div key={k}>
                    <Text fw={600} mb={6} size="sm">
                      {k === 'before' ? 'Antes' : 'Depois'}
                    </Text>
                    <ScrollArea.Autosize mah="60vh">
                      <Code block>{json(selected[k])}</Code>
                    </ScrollArea.Autosize>
                  </div>
                ))}
              </SimpleGrid>
            )}
          </Stack>
        )}
      </Modal>
    </>
  )
}
