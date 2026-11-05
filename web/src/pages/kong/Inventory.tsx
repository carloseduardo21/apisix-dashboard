import { useMemo, useState } from 'react'
import {
  ActionIcon,
  Affix,
  Badge,
  Button,
  Card,
  Checkbox,
  Collapse,
  Group,
  Pagination,
  Paper,
  SegmentedControl,
  SimpleGrid,
  Stack,
  Table,
  Text,
  TextInput,
  ThemeIcon,
  Title,
  Tooltip,
  Transition,
  UnstyledButton,
} from '@mantine/core'
import {
  IconAlertTriangle,
  IconChevronDown,
  IconChevronRight,
  IconEye,
  IconPlugConnected,
  IconRoute,
  IconSearch,
  IconServer,
  IconUsers,
  IconWorld,
  type Icon,
} from '@tabler/icons-react'
import { EmptyState } from '../../components/EmptyState'
import { ActionBadge, NoteList, PluginBadges } from './PreviewModal'
import { groupAction, type Action, type Plan, type PlanItem, type Selection } from './types'
import { Trans, useTranslation } from 'react-i18next'
import { noteText } from '../../i18n/format'

type Filter = 'all' | 'pending' | 'migrated' | 'blocked'

function matches(filter: Filter, a: Action) {
  switch (filter) {
    case 'pending':
      return a === 'create' || a === 'update' || a === 'overwrite' || a === 'conflict'
    case 'migrated':
      return a === 'unchanged'
    case 'blocked':
      return a === 'blocked'
  }
  return true
}

function Stat({
  icon: I,
  label,
  total,
  migrated,
  blocked,
}: {
  icon: Icon
  label: string
  total: number
  migrated: number
  blocked: number
}) {
  const { t } = useTranslation()
  return (
    <Card padding="md">
      <Group gap="sm" wrap="nowrap">
        <ThemeIcon size={38} radius="md" variant="light">
          <I size={20} stroke={1.6} />
        </ThemeIcon>
        <div>
          <Text size="xs" c="dimmed" tt="uppercase" fw={700}>
            {label}
          </Text>
          <Text fw={700} fz={22} lh={1.1}>
            {total}
          </Text>
        </div>
      </Group>
      <Group gap={6} mt="sm">
        <Badge size="sm" variant="light" color="teal">
          {t('kong.inv.migrated', { count: migrated })}
        </Badge>
        {blocked > 0 && (
          <Badge size="sm" variant="light" color="red">
            {t('kong.inv.blocked', { count: blocked })}
          </Badge>
        )}
      </Group>
    </Card>
  )
}

function Notes({ items }: { items: PlanItem[] }) {
  const notes = items.flatMap((i) =>
    i.notes.map((n) => ({ ...n, prefix: items.length > 1 ? kindPrefix(i) : undefined })),
  )
  return <NoteList notes={notes} />
}

const kindPrefix = (i: PlanItem) => (i.kind === 'upstream' ? 'upstream: ' : i.kind === 'service' ? 'service: ' : '')

interface ServiceGroup {
  service: PlanItem
  upstream?: PlanItem
  routes: PlanItem[]
}

function ServiceRow({
  g,
  selection,
  toggleService,
  toggleRoute,
}: {
  g: ServiceGroup
  selection: Selection
  toggleService: (id: string) => void
  toggleRoute: (id: string) => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const all = [g.service, ...(g.upstream ? [g.upstream] : []), ...g.routes]
  const action = groupAction(all)
  const svcSelected = selection.services.includes(g.service.kong_id)
  const someRoutes = g.routes.some((r) => selection.routes.includes(r.kong_id))
  const warn = all.some((i) => i.notes.some((n) => n.level === 'warn'))
  return (
    <Paper withBorder p="sm">
      <Group wrap="nowrap" gap="sm" align="flex-start">
        <Checkbox
          mt={4}
          checked={svcSelected}
          indeterminate={!svcSelected && someRoutes}
          onChange={() => toggleService(g.service.kong_id)}
          aria-label={t('kong.inv.select', { name: g.service.kong_name })}
        />
        <ActionIcon
          variant="subtle"
          color="gray"
          onClick={() => setOpen((o) => !o)}
          aria-label={t('kong.inv.details')}
          mt={1}
        >
          {open ? <IconChevronDown size={18} /> : <IconChevronRight size={18} />}
        </ActionIcon>
        <UnstyledButton
          aria-expanded={open}
          style={{ flex: 1, minWidth: 0, textAlign: 'left' }}
          onClick={() => setOpen((o) => !o)}
        >
          <Group justify="space-between" wrap="nowrap" gap="sm">
            <div style={{ minWidth: 0 }}>
              <Group gap={6} wrap="nowrap">
                <Text fw={700} truncate>
                  {g.service.kong_name}
                </Text>
                <Badge size="xs" variant="outline" color="gray">
                  {t('kong.inv.routes_count', { count: g.routes.length })}
                </Badge>
              </Group>
              <Text size="xs" c="dimmed" ff="monospace" truncate>
                {g.service.detail}
                {g.upstream?.detail ? ` → ${g.upstream.detail}` : ''}
              </Text>
            </div>
            <Group gap={6} wrap="nowrap">
              {warn && (
                <Tooltip label={t('kong.inv.has_warnings')}>
                  <IconAlertTriangle size={16} color="var(--mantine-color-orange-6)" />
                </Tooltip>
              )}
              <ActionBadge action={action} />
            </Group>
          </Group>
          <PluginBadges item={g.service} />
        </UnstyledButton>
      </Group>
      <Collapse expanded={open}>
        <Stack gap="sm" mt="sm" ml={{ base: 0, sm: 64 }}>
          <Notes items={[g.service, ...(g.upstream ? [g.upstream] : [])]} />
          {g.routes.length > 0 && (
            <Table.ScrollContainer minWidth={560}>
              <Table verticalSpacing={6} fz="sm" highlightOnHover>
                <Table.Thead>
                  <Table.Tr>
                    <Table.Th w={36} />
                    <Table.Th>{t('kong.inv.col_route')}</Table.Th>
                    <Table.Th>{t('columns.plugins')}</Table.Th>
                    <Table.Th w={110}>{t('common.status')}</Table.Th>
                  </Table.Tr>
                </Table.Thead>
                <Table.Tbody>
                  {g.routes.map((r) => (
                    <Table.Tr key={r.kong_id} style={{ verticalAlign: 'top' }}>
                      <Table.Td>
                        <Checkbox
                          checked={svcSelected || selection.routes.includes(r.kong_id)}
                          disabled={svcSelected}
                          onChange={() => toggleRoute(r.kong_id)}
                          aria-label={t('kong.inv.select', { name: r.kong_name })}
                        />
                      </Table.Td>
                      <Table.Td>
                        <Text size="sm" fw={600}>
                          {r.kong_name}
                        </Text>
                        <Text size="xs" c="dimmed" ff="monospace">
                          {r.detail}
                        </Text>
                        {r.notes.length > 0 && (
                          <div style={{ marginTop: 6 }}>
                            <NoteList notes={r.notes} />
                          </div>
                        )}
                      </Table.Td>
                      <Table.Td>
                        <PluginBadges item={r} />
                      </Table.Td>
                      <Table.Td>
                        <ActionBadge action={r.action} size="xs" />
                      </Table.Td>
                    </Table.Tr>
                  ))}
                </Table.Tbody>
              </Table>
            </Table.ScrollContainer>
          )}
        </Stack>
      </Collapse>
    </Paper>
  )
}

const PAGE = 25

function ItemTable({
  items,
  extra,
  selected,
  toggle,
  setMany,
  label,
}: {
  items: PlanItem[]
  extra?: (i: PlanItem) => React.ReactNode
  selected: string[]
  toggle: (id: string) => void
  setMany: (ids: string[], on: boolean) => void
  label: string
}) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const pages = Math.max(1, Math.ceil(items.length / PAGE))
  const shown = items.slice((Math.min(page, pages) - 1) * PAGE, Math.min(page, pages) * PAGE)
  const ids = items.map((i) => i.kong_id)
  const allOn = ids.length > 0 && ids.every((id) => selected.includes(id))
  const someOn = ids.some((id) => selected.includes(id))
  return (
    <>
      <Table.ScrollContainer minWidth={560}>
        <Table verticalSpacing={6} fz="sm" highlightOnHover>
          <Table.Thead>
            <Table.Tr>
              <Table.Th w={36}>
                <Tooltip label={t('kong.inv.select_all_filtered', { count: items.length })}>
                  <Checkbox
                    checked={allOn}
                    indeterminate={!allOn && someOn}
                    onChange={() => setMany(ids, !allOn)}
                    aria-label={t('kong.inv.select_all')}
                  />
                </Tooltip>
              </Table.Th>
              <Table.Th>{label}</Table.Th>
              <Table.Th>{t('kong.inv.details')}</Table.Th>
              <Table.Th w={110}>{t('common.status')}</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {shown.map((it) => (
              <Table.Tr key={it.kong_id} style={{ verticalAlign: 'top' }}>
                <Table.Td>
                  <Checkbox
                    checked={selected.includes(it.kong_id)}
                    onChange={() => toggle(it.kong_id)}
                    aria-label={t('kong.inv.select', { name: it.kong_name })}
                  />
                </Table.Td>
                <Table.Td>
                  <Text size="sm" fw={600}>
                    {it.kong_name}
                  </Text>
                  {it.id && it.id !== it.kong_name && (
                    <Text size="xs" c="dimmed" ff="monospace">
                      {it.resource}/{it.id}
                    </Text>
                  )}
                  {it.notes.length > 0 && (
                    <div style={{ marginTop: 6 }}>
                      <NoteList notes={it.notes} />
                    </div>
                  )}
                </Table.Td>
                <Table.Td>
                  <Stack gap={4}>
                    {it.detail && (
                      <Text size="xs" c="dimmed">
                        {it.detail}
                      </Text>
                    )}
                    <PluginBadges item={it} />
                    {extra?.(it)}
                  </Stack>
                </Table.Td>
                <Table.Td>
                  <ActionBadge action={it.action} size="xs" />
                </Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      </Table.ScrollContainer>
      {pages > 1 && <Pagination total={pages} value={Math.min(page, pages)} onChange={setPage} size="sm" mt="sm" />}
    </>
  )
}

const empty = (): Selection => ({ services: [], routes: [], consumers: [], globals: [] })

const count = (items: PlanItem[]) => ({
  total: items.length,
  migrated: items.filter((i) => i.action === 'unchanged').length,
  blocked: items.filter((i) => i.action === 'blocked').length,
})

/** Kong configuration grouped for step-by-step selection. */
export function Inventory({ plan, onPreview }: { plan: Plan; onPreview: (s: Selection) => void }) {
  const { t } = useTranslation()
  const [selection, setSelection] = useState<Selection>(empty)
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState<Filter>('all')

  const { groups, orphans, consumers, credentials, globals } = useMemo(() => {
    const byKind = (k: PlanItem['kind']) => plan.items.filter((i) => i.kind === k)
    const serviceGroups: ServiceGroup[] = byKind('service').map((s) => ({
      service: s,
      upstream: plan.items.find((i) => i.kind === 'upstream' && i.kong_id === s.kong_id),
      routes: plan.items.filter((i) => i.kind === 'route' && i.parent === s.kong_id),
    }))
    return {
      groups: serviceGroups,
      orphans: plan.items.filter((i) => i.kind === 'route' && !i.parent),
      consumers: byKind('consumer'),
      credentials: byKind('credential'),
      globals: byKind('global_plugin'),
    }
  }, [plan])

  const q = query.trim().toLowerCase()
  const hit = (i: PlanItem) =>
    !q ||
    i.kong_name.toLowerCase().includes(q) ||
    (i.detail ?? '').toLowerCase().includes(q) ||
    i.id.toLowerCase().includes(q)
  const credsOf = (c: PlanItem) => credentials.filter((x) => x.parent === c.kong_id)

  const visibleGroups = groups.filter((g) => {
    const all = [g.service, ...(g.upstream ? [g.upstream] : []), ...g.routes]
    return all.some(hit) && matches(filter, groupAction(all))
  })
  const visibleOrphans = orphans.filter((i) => hit(i) && matches(filter, i.action))
  const visibleConsumers = consumers.filter(
    (c) => (hit(c) || credsOf(c).some(hit)) && matches(filter, groupAction([c, ...credsOf(c)])),
  )
  const visibleGlobals = globals.filter((i) => hit(i) && matches(filter, i.action))

  const toggleIn = (key: keyof Selection, id: string) =>
    setSelection((s) => ({ ...s, [key]: s[key].includes(id) ? s[key].filter((x) => x !== id) : [...s[key], id] }))
  const setMany = (key: keyof Selection) => (ids: string[], on: boolean) =>
    setSelection((s) => ({
      ...s,
      [key]: on ? [...new Set([...s[key], ...ids])] : s[key].filter((x) => !ids.includes(x)),
    }))
  const toggleService = (id: string) => {
    const routeIds = groups.find((g) => g.service.kong_id === id)?.routes.map((r) => r.kong_id) ?? []
    setSelection((s) => ({
      ...s,
      services: s.services.includes(id) ? s.services.filter((x) => x !== id) : [...s.services, id],
      // Selecting the service brings every route; drop the individual picks.
      routes: s.routes.filter((r) => !routeIds.includes(r)),
    }))
  }

  const nSel =
    selection.services.length + selection.routes.length + selection.consumers.length + selection.globals.length

  return (
    <Stack pb={nSel ? 80 : 0}>
      <SimpleGrid cols={{ base: 2, md: 4 }}>
        <Stat icon={IconServer} label={t('kong.inv.stat_services')} {...count(groups.map((g) => g.service))} />
        <Stat
          icon={IconRoute}
          label={t('kong.inv.stat_routes')}
          {...count(plan.items.filter((i) => i.kind === 'route'))}
        />
        <Stat icon={IconUsers} label={t('kong.inv.stat_consumers')} {...count(consumers)} />
        <Stat icon={IconWorld} label={t('kong.inv.stat_globals')} {...count(globals)} />
      </SimpleGrid>

      <Group gap="sm" wrap="wrap">
        <TextInput
          placeholder={t('kong.inv.search')}
          leftSection={<IconSearch size={16} />}
          value={query}
          onChange={(e) => setQuery(e.currentTarget.value)}
          style={{ flex: 1, minWidth: 240 }}
        />
        <SegmentedControl
          value={filter}
          onChange={(v) => setFilter(v)}
          data={[
            { value: 'all', label: t('kong.inv.filter_all') },
            { value: 'pending', label: t('kong.inv.filter_pending') },
            { value: 'migrated', label: t('kong.inv.filter_migrated') },
            { value: 'blocked', label: t('kong.inv.filter_blocked') },
          ]}
        />
      </Group>

      <Card>
        <Title order={4} mb="sm">
          {t('kong.inv.services_title')}
        </Title>
        <Text size="sm" c="dimmed" mb="md">
          {t('kong.inv.services_hint')}
        </Text>
        <Stack gap="xs">
          {visibleGroups.length === 0 && (
            <EmptyState icon={IconServer} title={t('kong.inv.no_services')} description={t('kong.inv.no_match')} />
          )}
          {visibleGroups.map((g) => (
            <ServiceRow
              key={g.service.kong_id}
              g={g}
              selection={selection}
              toggleService={toggleService}
              toggleRoute={(id) => toggleIn('routes', id)}
            />
          ))}
        </Stack>
        {visibleOrphans.length > 0 && (
          <>
            <Title order={5} mt="lg" mb="xs">
              {t('kong.inv.orphan_routes')}
            </Title>
            <ItemTable
              items={visibleOrphans}
              label={t('kong.inv.col_route')}
              selected={selection.routes}
              toggle={(id) => toggleIn('routes', id)}
              setMany={setMany('routes')}
            />
          </>
        )}
      </Card>

      <Card>
        <Title order={4} mb="sm">
          {t('kong.inv.consumers_title')}
        </Title>
        <Text size="sm" c="dimmed" mb="md">
          <Trans i18nKey="kong.inv.consumers_hint" components={{ b: <b /> }} />
        </Text>
        {visibleConsumers.length === 0 ? (
          <EmptyState icon={IconUsers} title={t('kong.inv.no_consumers')} description={t('kong.inv.no_match')} />
        ) : (
          <ItemTable
            items={visibleConsumers}
            label={t('kong.inv.stat_consumers')}
            selected={selection.consumers}
            toggle={(id) => toggleIn('consumers', id)}
            setMany={setMany('consumers')}
            // oxlint-disable-next-line react/no-unstable-nested-components -- render prop invoked as extra?.(item), not mounted as a component
            extra={(c) => {
              const creds = credsOf(c)
              if (!creds.length) return null
              return (
                <Stack gap={4}>
                  <Group gap={4}>
                    {creds.map((x) => (
                      <Tooltip
                        key={x.kong_id}
                        label={(() => {
                          const n = x.notes.find((note) => note.level !== 'info')
                          return n ? noteText(n) : (x.detail ?? t('kong.inv.credential'))
                        })()}
                        multiline
                        maw={320}
                      >
                        <Badge
                          size="xs"
                          variant="light"
                          color={x.action === 'blocked' ? 'red' : x.action === 'unchanged' ? 'teal' : 'indigo'}
                        >
                          {x.kong_name.split(' · ').pop()}
                        </Badge>
                      </Tooltip>
                    ))}
                  </Group>
                </Stack>
              )
            }}
          />
        )}
      </Card>

      <Card>
        <Title order={4} mb="sm">
          {t('kong.inv.globals_title')}
        </Title>
        <Text size="sm" c="dimmed" mb="md">
          {t('kong.inv.globals_hint')}
        </Text>
        {visibleGlobals.length === 0 ? (
          <EmptyState icon={IconPlugConnected} title={t('kong.inv.no_globals')} />
        ) : (
          <ItemTable
            items={visibleGlobals}
            label={t('kong.inv.col_plugin')}
            selected={selection.globals}
            toggle={(id) => toggleIn('globals', id)}
            setMany={setMany('globals')}
          />
        )}
      </Card>

      <Affix position={{ bottom: 20, left: 0, right: 0 }} zIndex={150}>
        <Transition transition="slide-up" mounted={nSel > 0}>
          {(styles) => (
            <Paper shadow="lg" withBorder p="sm" mx="auto" maw={760} style={{ ...styles, width: 'calc(100% - 32px)' }}>
              <Group justify="space-between" wrap="wrap" gap="sm">
                <Text size="sm">
                  <b>{t('kong.inv.selected')}</b>{' '}
                  {(
                    [
                      [t('kong.inv.stat_services'), selection.services.length],
                      [t('kong.inv.stat_routes'), selection.routes.length],
                      [t('kong.inv.stat_consumers'), selection.consumers.length],
                      [t('kong.inv.stat_globals'), selection.globals.length],
                    ] as const
                  )
                    .filter(([, n]) => n > 0)
                    .map(([label, n]) => t('kong.inv.sel_item', { label, count: n }))
                    .join(' · ')}
                </Text>
                <Group gap="xs">
                  <Button variant="default" size="sm" onClick={() => setSelection(empty())}>
                    {t('kong.inv.clear')}
                  </Button>
                  <Button size="sm" leftSection={<IconEye size={16} />} onClick={() => onPreview(selection)}>
                    {t('kong.inv.preview')}
                  </Button>
                </Group>
              </Group>
            </Paper>
          )}
        </Transition>
      </Affix>
    </Stack>
  )
}
