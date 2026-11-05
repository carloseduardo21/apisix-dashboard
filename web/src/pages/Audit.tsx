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
import { useTranslation } from 'react-i18next'
import { dynamicText, formatDate, formatDateTime, formatTime } from '../i18n/format'

interface Page {
  items: AuditEvent[]
  next_before?: number
}

/** Audit action name in the current language (unknown actions are shown as-is). */
export const actionLabel = (a: string) => dynamicText(`audit.actions.${a}`, a)

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
  const { t } = useTranslation()
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
        title={t('audit.title')}
        description={t('audit.description')}
        actions={
          <Button
            variant="default"
            leftSection={<IconRefresh size={16} />}
            onClick={() => q.refetch()}
            loading={q.isFetching && !q.isFetchingNextPage}
          >
            {t('audit.refresh')}
          </Button>
        }
      />

      <Card padding={0}>
        <Group justify="space-between" p="md" gap="sm">
          <SegmentedControl
            value={filter}
            onChange={(v) => setFilter(v)}
            data={[
              { value: 'all', label: t('audit.filter_all') },
              { value: 'changes', label: t('audit.filter_changes') },
              { value: 'auth', label: t('audit.filter_auth') },
              { value: 'failures', label: t('audit.filter_failures') },
            ]}
          />
          <TextInput
            placeholder={t('audit.search')}
            leftSection={<IconSearch size={16} />}
            value={search}
            onChange={(e) => setSearch(e.currentTarget.value)}
            w={300}
          />
        </Group>
        {events.length === 0 && !q.isPending ? (
          <EmptyState icon={IconHistory} title={t('audit.empty')} description={t('audit.empty_desc')} />
        ) : (
          <Table.ScrollContainer minWidth={900}>
            <Table highlightOnHover verticalSpacing="sm" horizontalSpacing="md">
              <Table.Thead bg="var(--mantine-color-default-hover)">
                <Table.Tr>
                  <Table.Th>{t('audit.col_when')}</Table.Th>
                  <Table.Th>{t('audit.col_user')}</Table.Th>
                  <Table.Th>{t('audit.col_action')}</Table.Th>
                  <Table.Th>{t('audit.col_resource')}</Table.Th>
                  <Table.Th>IP</Table.Th>
                  <Table.Th>{t('audit.col_detail')}</Table.Th>
                </Table.Tr>
              </Table.Thead>
              <Table.Tbody>
                {events.map((e) => (
                  <Table.Tr key={e.id} style={{ cursor: 'pointer' }} onClick={() => setSelected(e)}>
                    <Table.Td style={{ whiteSpace: 'nowrap' }}>
                      <Text size="sm">{formatDate(e.ts)}</Text>
                      <Text size="xs" c="dimmed">
                        {formatTime(e.ts)}
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
                        {e.outcome === 'failure'
                          ? t('audit.failed', { action: actionLabel(e.action) })
                          : actionLabel(e.action)}
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
              {t('audit.load_more')}
            </Button>
          </Group>
        )}
      </Card>

      <Modal
        opened={!!selected}
        onClose={() => setSelected(null)}
        size="90%"
        title={<Text fw={600}>{t('audit.event', { id: selected?.id })}</Text>}
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
                  {formatDateTime(selected.ts)} · {selected.ip || t('common.no_ip')}
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
                      {k === 'before' ? t('audit.before') : t('audit.after')}
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
