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

function Stat({ icon: I, label, total, migrated, blocked }: { icon: Icon; label: string; total: number; migrated: number; blocked: number }) {
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
          {migrated} migrados
        </Badge>
        {blocked > 0 && (
          <Badge size="sm" variant="light" color="red">
            {blocked} bloqueados
          </Badge>
        )}
      </Group>
    </Card>
  )
}

function Notes({ items }: { items: PlanItem[] }) {
  const notes = items.flatMap((i) => i.notes.map((n) => ({ ...n, message: items.length > 1 ? `${kindPrefix(i)}${n.message}` : n.message })))
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
          aria-label={`Selecionar ${g.service.kong_name}`}
        />
        <ActionIcon variant="subtle" color="gray" onClick={() => setOpen((o) => !o)} aria-label="Detalhes" mt={1}>
          {open ? <IconChevronDown size={18} /> : <IconChevronRight size={18} />}
        </ActionIcon>
        <div style={{ flex: 1, minWidth: 0, cursor: 'pointer' }} onClick={() => setOpen((o) => !o)}>
          <Group justify="space-between" wrap="nowrap" gap="sm">
            <div style={{ minWidth: 0 }}>
              <Group gap={6} wrap="nowrap">
                <Text fw={700} truncate>
                  {g.service.kong_name}
                </Text>
                <Badge size="xs" variant="outline" color="gray">
                  {g.routes.length} {g.routes.length === 1 ? 'rota' : 'rotas'}
                </Badge>
              </Group>
              <Text size="xs" c="dimmed" ff="monospace" truncate>
                {g.service.detail}
                {g.upstream?.detail ? ` → ${g.upstream.detail}` : ''}
              </Text>
            </div>
            <Group gap={6} wrap="nowrap">
              {warn && (
                <Tooltip label="Há avisos: abra para ver">
                  <IconAlertTriangle size={16} color="var(--mantine-color-orange-6)" />
                </Tooltip>
              )}
              <ActionBadge action={action} />
            </Group>
          </Group>
          <PluginBadges item={g.service} />
        </div>
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
                    <Table.Th>Rota</Table.Th>
                    <Table.Th>Plugins</Table.Th>
                    <Table.Th w={110}>Status</Table.Th>
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
                          aria-label={`Selecionar ${r.kong_name}`}
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
                <Tooltip label={`Selecionar todos os ${items.length} (filtrados)`}>
                  <Checkbox checked={allOn} indeterminate={!allOn && someOn} onChange={() => setMany(ids, !allOn)} aria-label="Selecionar todos" />
                </Tooltip>
              </Table.Th>
              <Table.Th>{label}</Table.Th>
              <Table.Th>Detalhes</Table.Th>
              <Table.Th w={110}>Status</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {shown.map((it) => (
              <Table.Tr key={it.kong_id} style={{ verticalAlign: 'top' }}>
                <Table.Td>
                  <Checkbox checked={selected.includes(it.kong_id)} onChange={() => toggle(it.kong_id)} aria-label={`Selecionar ${it.kong_name}`} />
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

const plural = (n: number, one: string, many: string) => n > 0 && `${n} ${n === 1 ? one : many}`

const empty = (): Selection => ({ services: [], routes: [], consumers: [], globals: [] })

/** Kong configuration grouped for step-by-step selection. */
export function Inventory({ plan, onPreview }: { plan: Plan; onPreview: (s: Selection) => void }) {
  const [selection, setSelection] = useState<Selection>(empty)
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState<Filter>('all')

  const { groups, orphans, consumers, credentials, globals } = useMemo(() => {
    const byKind = (k: PlanItem['kind']) => plan.items.filter((i) => i.kind === k)
    const groups: ServiceGroup[] = byKind('service').map((s) => ({
      service: s,
      upstream: plan.items.find((i) => i.kind === 'upstream' && i.kong_id === s.kong_id),
      routes: plan.items.filter((i) => i.kind === 'route' && i.parent === s.kong_id),
    }))
    return {
      groups,
      orphans: plan.items.filter((i) => i.kind === 'route' && !i.parent),
      consumers: byKind('consumer'),
      credentials: byKind('credential'),
      globals: byKind('global_plugin'),
    }
  }, [plan])

  const q = query.trim().toLowerCase()
  const hit = (i: PlanItem) => !q || i.kong_name.toLowerCase().includes(q) || (i.detail ?? '').toLowerCase().includes(q) || i.id.toLowerCase().includes(q)
  const credsOf = (c: PlanItem) => credentials.filter((x) => x.parent === c.kong_id)

  const visibleGroups = groups.filter((g) => {
    const all = [g.service, ...(g.upstream ? [g.upstream] : []), ...g.routes]
    return all.some(hit) && matches(filter, groupAction(all))
  })
  const visibleOrphans = orphans.filter((i) => hit(i) && matches(filter, i.action))
  const visibleConsumers = consumers.filter((c) => (hit(c) || credsOf(c).some(hit)) && matches(filter, groupAction([c, ...credsOf(c)])))
  const visibleGlobals = globals.filter((i) => hit(i) && matches(filter, i.action))

  const toggleIn = (key: keyof Selection, id: string) =>
    setSelection((s) => ({ ...s, [key]: s[key].includes(id) ? s[key].filter((x) => x !== id) : [...s[key], id] }))
  const setMany = (key: keyof Selection) => (ids: string[], on: boolean) =>
    setSelection((s) => ({ ...s, [key]: on ? [...new Set([...s[key], ...ids])] : s[key].filter((x) => !ids.includes(x)) }))
  const toggleService = (id: string) => {
    const routeIds = groups.find((g) => g.service.kong_id === id)?.routes.map((r) => r.kong_id) ?? []
    setSelection((s) => ({
      ...s,
      services: s.services.includes(id) ? s.services.filter((x) => x !== id) : [...s.services, id],
      // Selecting the service brings every route; drop the individual picks.
      routes: s.routes.filter((r) => !routeIds.includes(r)),
    }))
  }

  const count = (items: PlanItem[]) => ({
    total: items.length,
    migrated: items.filter((i) => i.action === 'unchanged').length,
    blocked: items.filter((i) => i.action === 'blocked').length,
  })
  const nSel = selection.services.length + selection.routes.length + selection.consumers.length + selection.globals.length

  return (
    <Stack pb={nSel ? 80 : 0}>
      <SimpleGrid cols={{ base: 2, md: 4 }}>
        <Stat icon={IconServer} label="Services" {...count(groups.map((g) => g.service))} />
        <Stat icon={IconRoute} label="Rotas" {...count(plan.items.filter((i) => i.kind === 'route'))} />
        <Stat icon={IconUsers} label="Consumers" {...count(consumers)} />
        <Stat icon={IconWorld} label="Plugins globais" {...count(globals)} />
      </SimpleGrid>

      <Group gap="sm" wrap="wrap">
        <TextInput
          placeholder="Buscar por nome, host, path ou ID"
          leftSection={<IconSearch size={16} />}
          value={query}
          onChange={(e) => setQuery(e.currentTarget.value)}
          style={{ flex: 1, minWidth: 240 }}
        />
        <SegmentedControl
          value={filter}
          onChange={(v) => setFilter(v)}
          data={[
            { value: 'all', label: 'Todos' },
            { value: 'pending', label: 'A migrar' },
            { value: 'migrated', label: 'Migrados' },
            { value: 'blocked', label: 'Bloqueados' },
          ]}
        />
      </Group>

      <Card>
        <Title order={4} mb="sm">
          Services e rotas
        </Title>
        <Text size="sm" c="dimmed" mb="md">
          Selecionar um service leva o upstream, os plugins e todas as rotas dele. Para migrar só algumas rotas, abra o service e marque as rotas.
        </Text>
        <Stack gap="xs">
          {visibleGroups.length === 0 && <EmptyState icon={IconServer} title="Nenhum service" description="Nada corresponde ao filtro." />}
          {visibleGroups.map((g) => (
            <ServiceRow key={g.service.kong_id} g={g} selection={selection} toggleService={toggleService} toggleRoute={(id) => toggleIn('routes', id)} />
          ))}
        </Stack>
        {visibleOrphans.length > 0 && (
          <>
            <Title order={5} mt="lg" mb="xs">
              Rotas sem service
            </Title>
            <ItemTable items={visibleOrphans} label="Rota" selected={selection.routes} toggle={(id) => toggleIn('routes', id)} setMany={setMany('routes')} />
          </>
        )}
      </Card>

      <Card>
        <Title order={4} mb="sm">
          Consumers e credenciais
        </Title>
        <Text size="sm" c="dimmed" mb="md">
          Os grupos de ACL viram o label <b>groups</b> do consumer (lido pelo plugin acl). Migre os consumers antes de ativar as rotas que os autenticam.
        </Text>
        {visibleConsumers.length === 0 ? (
          <EmptyState icon={IconUsers} title="Nenhum consumer" description="Nada corresponde ao filtro." />
        ) : (
          <ItemTable
            items={visibleConsumers}
            label="Consumer"
            selected={selection.consumers}
            toggle={(id) => toggleIn('consumers', id)}
            setMany={setMany('consumers')}
            extra={(c) => {
              const creds = credsOf(c)
              if (!creds.length) return null
              return (
                <Stack gap={4}>
                  <Group gap={4}>
                    {creds.map((x) => (
                      <Tooltip key={x.kong_id} label={x.notes.find((n) => n.level !== 'info')?.message ?? x.detail ?? 'credencial'} multiline maw={320}>
                        <Badge size="xs" variant="light" color={x.action === 'blocked' ? 'red' : x.action === 'unchanged' ? 'teal' : 'indigo'}>
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
          Plugins globais
        </Title>
        <Text size="sm" c="dimmed" mb="md">
          Viram global rules no APISIX e passam a valer para todas as rotas, inclusive as que ainda não vieram do Kong.
        </Text>
        {visibleGlobals.length === 0 ? (
          <EmptyState icon={IconPlugConnected} title="Nenhum plugin global" />
        ) : (
          <ItemTable items={visibleGlobals} label="Plugin" selected={selection.globals} toggle={(id) => toggleIn('globals', id)} setMany={setMany('globals')} />
        )}
      </Card>

      <Affix position={{ bottom: 20, left: 0, right: 0 }} zIndex={150}>
        <Transition transition="slide-up" mounted={nSel > 0}>
          {(styles) => (
            <Paper shadow="lg" withBorder p="sm" mx="auto" maw={760} style={{ ...styles, width: 'calc(100% - 32px)' }}>
              <Group justify="space-between" wrap="wrap" gap="sm">
                <Text size="sm">
                  <b>Selecionado:</b>{' '}
                  {[
                    plural(selection.services.length, 'service', 'services'),
                    plural(selection.routes.length, 'rota', 'rotas'),
                    plural(selection.consumers.length, 'consumer', 'consumers'),
                    plural(selection.globals.length, 'plugin global', 'plugins globais'),
                  ]
                    .filter(Boolean)
                    .join(' · ')}
                </Text>
                <Group gap="xs">
                  <Button variant="default" size="sm" onClick={() => setSelection(empty())}>
                    Limpar
                  </Button>
                  <Button size="sm" leftSection={<IconEye size={16} />} onClick={() => onPreview(selection)}>
                    Pré-visualizar
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
