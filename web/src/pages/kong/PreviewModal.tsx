import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Accordion,
  Alert,
  Badge,
  Button,
  Checkbox,
  Code,
  Group,
  Loader,
  Modal,
  Progress,
  ScrollArea,
  SimpleGrid,
  Stack,
  Table,
  Text,
  Timeline,
  Tooltip,
} from '@mantine/core'
import { useMediaQuery } from '@mantine/hooks'
import {
  IconAlertCircle,
  IconAlertTriangle,
  IconArrowRight,
  IconCheck,
  IconInfoCircle,
  IconPlayerPlay,
  IconRotate,
  IconX,
} from '@tabler/icons-react'
import { api } from '../../api'
import { ACTION, KIND_LABEL, writes, type Job, type Note, type Plan, type PlanItem, type Selection } from './types'

const NOTE_ICON = { info: IconInfoCircle, warn: IconAlertTriangle, error: IconAlertCircle }
const NOTE_COLOR = { info: 'indigo', warn: 'orange', error: 'red' }

export function NoteList({ notes }: { notes: Note[] }) {
  if (notes.length === 0) return null
  return (
    <Stack gap={4}>
      {notes.map((n, i) => {
        const I = NOTE_ICON[n.level]
        return (
          // oxlint-disable-next-line react/no-array-index-key -- read-only list from the server, never reordered
          <Group key={i} gap={6} wrap="nowrap" align="flex-start">
            <I
              size={15}
              color={`var(--mantine-color-${NOTE_COLOR[n.level]}-6)`}
              style={{ flexShrink: 0, marginTop: 2 }}
            />
            <Text size="sm">{n.message}</Text>
          </Group>
        )
      })}
    </Stack>
  )
}

export function ActionBadge({ action, size = 'sm' }: { action: PlanItem['action']; size?: string }) {
  const a = ACTION[action]
  return (
    <Tooltip label={a.hint}>
      <Badge color={a.color} variant="light" size={size}>
        {a.label}
      </Badge>
    </Tooltip>
  )
}

export function PluginBadges({ item }: { item: PlanItem }) {
  if (!item.plugins?.length) return null
  const color = { ok: 'gray', warn: 'orange', error: 'red', ignored: 'gray', disabled: 'gray' }
  return (
    <Group gap={4}>
      {item.plugins.map((p, i) => (
        <Tooltip
          // oxlint-disable-next-line react/no-array-index-key -- read-only list from the server, never reordered
          key={i}
          label={
            p.status === 'ignored'
              ? 'ignorado pela regra'
              : p.status === 'disabled'
                ? 'desabilitado no Kong'
                : p.apisix?.length
                  ? `vira ${p.apisix.join(', ')}${p.custom ? ' (regra custom)' : ''}`
                  : 'sem tradução'
          }
        >
          <Badge
            size="xs"
            variant={p.status === 'ignored' || p.status === 'disabled' ? 'outline' : 'light'}
            color={color[p.status]}
            td={p.status === 'disabled' ? 'line-through' : undefined}
          >
            {p.kong}
            {p.custom ? ' ★' : ''}
          </Badge>
        </Tooltip>
      ))}
    </Group>
  )
}

const json = (v: unknown) => JSON.stringify(v, null, 2)

function ItemDetail({ item }: { item: PlanItem }) {
  const diff = item.current && (item.action === 'update' || item.action === 'overwrite' || item.action === 'conflict')
  return (
    <Stack gap="sm">
      <NoteList notes={item.notes} />
      {item.plugins?.some((p) => p.apisix?.length) && (
        <Group gap={6}>
          {item.plugins
            .filter((p) => p.apisix?.length)
            .map((p) => (
              <Badge key={p.kong} variant="default" size="sm">
                {p.kong} → {(p.apisix ?? []).join(', ')}
              </Badge>
            ))}
        </Group>
      )}
      {item.body &&
        (diff ? (
          <SimpleGrid cols={{ base: 1, md: 2 }}>
            <div>
              <Text size="xs" c="dimmed" fw={700} tt="uppercase" mb={4}>
                Atual no APISIX
              </Text>
              <Code block>{json(item.current)}</Code>
            </div>
            <div>
              <Text size="xs" c="dimmed" fw={700} tt="uppercase" mb={4}>
                Vindo do Kong
              </Text>
              <Code block>{json(item.body)}</Code>
            </div>
          </SimpleGrid>
        ) : (
          <Code block>{json(item.body)}</Code>
        ))}
    </Stack>
  )
}

const OUTCOME = {
  success: { label: 'gravado', color: 'teal' },
  failure: { label: 'falhou', color: 'red' },
  rolled_back: { label: 'desfeito', color: 'orange' },
  skipped: { label: 'pulado', color: 'gray' },
}

function JobView({ job }: { job: Job }) {
  const pct = job.total ? (job.done / job.total) * 100 : 0
  return (
    <Stack>
      {job.status === 'running' && (
        <Group gap="sm">
          <Loader size="sm" />
          <Text fw={600}>
            Aplicando {job.done} de {job.total}…
          </Text>
        </Group>
      )}
      {job.status === 'success' && (
        <Alert color="teal" variant="light" icon={<IconCheck size={18} />} title="Migração concluída">
          {job.done} objetos gravados no APISIX e registrados na auditoria.
        </Alert>
      )}
      {job.status === 'failed' && (
        <Alert color="red" variant="light" icon={<IconRotate size={18} />} title="A migração falhou e foi desfeita">
          {job.error}
        </Alert>
      )}
      <Progress value={pct} color={job.status === 'failed' ? 'red' : 'teal'} animated={job.status === 'running'} />
      <Timeline bulletSize={20} lineWidth={2} active={job.log.length - 1}>
        {job.log.map((e, i) => (
          <Timeline.Item
            // oxlint-disable-next-line react/no-array-index-key -- read-only list from the server, never reordered
            key={i}
            color={NOTE_COLOR[e.level]}
            bullet={e.level === 'error' ? <IconX size={11} /> : <IconCheck size={11} />}
          >
            <Text size="sm">{e.message}</Text>
            <Text size="xs" c="dimmed">
              {new Date(e.ts).toLocaleTimeString('pt-BR')}
            </Text>
          </Timeline.Item>
        ))}
      </Timeline>
      <Table.ScrollContainer minWidth={560}>
        <Table verticalSpacing={6} fz="sm">
          <Table.Thead>
            <Table.Tr>
              <Table.Th>Objeto</Table.Th>
              <Table.Th>APISIX</Table.Th>
              <Table.Th>Ação</Table.Th>
              <Table.Th>Resultado</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {job.results.map((r, i) => (
              // oxlint-disable-next-line react/no-array-index-key -- read-only list from the server, never reordered
              <Table.Tr key={i}>
                <Table.Td>
                  <Text size="xs" c="dimmed">
                    {KIND_LABEL[r.kind]}
                  </Text>
                  {r.kong_name}
                </Table.Td>
                <Table.Td>
                  <Code>{r.path}</Code>
                </Table.Td>
                <Table.Td>
                  <ActionBadge action={r.action} size="xs" />
                </Table.Td>
                <Table.Td>
                  <Badge size="xs" variant="light" color={OUTCOME[r.outcome].color}>
                    {OUTCOME[r.outcome].label}
                  </Badge>
                  {r.error && (
                    <Text size="xs" c="red" mt={2}>
                      {r.error}
                    </Text>
                  )}
                </Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      </Table.ScrollContainer>
    </Stack>
  )
}

/** Shows what a selection will write to APISIX and applies it. */
export function PreviewModal({
  selection,
  opened,
  onClose,
}: {
  selection: Selection | null
  opened: boolean
  onClose: () => void
}) {
  const qc = useQueryClient()
  const mobile = useMediaQuery('(max-width: 48em)')
  const [confirmed, setConfirmed] = useState(false)
  const [jobId, setJobId] = useState<string | null>(null)
  const [error, setError] = useState('')

  // Reset the dialog each time it opens (adjusting state on a prop change, no effect needed).
  const [wasOpened, setWasOpened] = useState(opened)
  if (opened !== wasOpened) {
    setWasOpened(opened)
    if (opened) {
      setConfirmed(false)
      setJobId(null)
      setError('')
    }
  }

  const plan = useQuery({
    queryKey: ['kong', 'plan', selection],
    queryFn: () => api<Plan>('POST', '/api/migration/kong/plan', selection),
    enabled: opened && !!selection && !jobId,
    staleTime: 0,
    gcTime: 0,
  })
  const job = useQuery({
    queryKey: ['kong', 'job'],
    queryFn: () => api<{ job: Job | null }>('GET', '/api/migration/kong/job'),
    enabled: !!jobId,
    refetchInterval: (q) => (q.state.data?.job?.status === 'running' ? 1000 : false),
  })
  const current = job.data?.job?.id === jobId ? job.data?.job : undefined
  // Depend on the status only: the job object changes on every poll.
  const status = current?.status
  useEffect(() => {
    if (status && status !== 'running') void qc.invalidateQueries({ queryKey: ['kong'] })
  }, [status, qc])

  const apply = useMutation({
    mutationFn: () => {
      if (!plan.data) throw new Error('A pré-visualização ainda não foi carregada')
      return api<Job>('POST', '/api/migration/kong/apply', { ...selection, hash: plan.data.hash })
    },
    onSuccess: (j) => setJobId(j.id),
    onError: (e) => setError(e.message),
  })

  const items = plan.data?.items ?? []
  const toWrite = items.filter((i) => writes(i.action))
  const counts = items.reduce<Record<string, number>>((acc, i) => {
    acc[i.action] = (acc[i.action] ?? 0) + 1
    return acc
  }, {})
  const skipped = items.filter((i) => i.action === 'blocked' || i.action === 'conflict').length

  return (
    <Modal
      opened={opened}
      onClose={onClose}
      size="90%"
      fullScreen={mobile}
      title={<Text fw={700}>{jobId ? 'Aplicando a migração' : 'Pré-visualização da migração'}</Text>}
    >
      {jobId ? (
        current ? (
          <JobView job={current} />
        ) : (
          <Loader />
        )
      ) : plan.isPending ? (
        <Group py="xl" justify="center">
          <Loader />
          <Text c="dimmed">Lendo o Kong e comparando com o APISIX…</Text>
        </Group>
      ) : plan.error ? (
        <Alert color="red" variant="light" icon={<IconAlertCircle size={16} />}>
          {plan.error.message}
        </Alert>
      ) : (
        <Stack>
          <Group gap="xs">
            {(['create', 'update', 'overwrite', 'unchanged', 'conflict', 'blocked'] as const)
              .filter((a) => counts[a])
              .map((a) => (
                <Badge key={a} color={ACTION[a].color} variant="light" size="lg">
                  {counts[a]} {ACTION[a].label}
                </Badge>
              ))}
          </Group>
          <Text size="sm" c="dimmed">
            Ordem de gravação: upstreams <IconArrowRight size={12} /> services <IconArrowRight size={12} /> consumers{' '}
            <IconArrowRight size={12} /> credenciais <IconArrowRight size={12} /> rotas <IconArrowRight size={12} />{' '}
            plugins globais. Se algo falhar, o que este lote gravou é desfeito.
          </Text>
          {skipped > 0 && (
            <Alert color="orange" variant="light" icon={<IconAlertTriangle size={16} />}>
              {skipped} {skipped === 1 ? 'item não será aplicado' : 'itens não serão aplicados'} (bloqueados ou em
              conflito). Os demais seguem normalmente.
            </Alert>
          )}
          <ScrollArea.Autosize mah="58vh" offsetScrollbars>
            <Accordion variant="separated" multiple chevronPosition="left">
              {items.map((it) => (
                <Accordion.Item key={it.kind + it.kong_id} value={it.kind + it.kong_id}>
                  <Accordion.Control>
                    <Group justify="space-between" wrap="nowrap" gap="sm">
                      <div style={{ minWidth: 0 }}>
                        <Group gap={6} wrap="nowrap">
                          <Badge size="xs" variant="outline" color="gray">
                            {KIND_LABEL[it.kind]}
                          </Badge>
                          <Text fw={600} size="sm" truncate>
                            {it.kong_name}
                          </Text>
                        </Group>
                        <Text size="xs" c="dimmed" ff="monospace" truncate>
                          {it.resource}/{it.id}
                        </Text>
                      </div>
                      <Group gap={6} wrap="nowrap">
                        {it.notes.some((n) => n.level === 'warn') && (
                          <IconAlertTriangle size={16} color="var(--mantine-color-orange-6)" />
                        )}
                        <ActionBadge action={it.action} />
                      </Group>
                    </Group>
                  </Accordion.Control>
                  <Accordion.Panel>
                    <ItemDetail item={it} />
                  </Accordion.Panel>
                </Accordion.Item>
              ))}
            </Accordion>
          </ScrollArea.Autosize>
          {error && (
            <Alert color="red" variant="light" icon={<IconAlertCircle size={16} />}>
              {error}
            </Alert>
          )}
          <Group justify="space-between" wrap="wrap">
            <Checkbox
              checked={confirmed}
              onChange={(e) => setConfirmed(e.currentTarget.checked)}
              disabled={toWrite.length === 0}
              label="Revisei o que será gravado no APISIX"
            />
            <Group>
              <Button variant="default" onClick={onClose}>
                Cancelar
              </Button>
              <Button
                leftSection={<IconPlayerPlay size={16} />}
                disabled={!confirmed || toWrite.length === 0}
                loading={apply.isPending}
                onClick={() => apply.mutate()}
              >
                {toWrite.length === 0
                  ? 'Nada a aplicar'
                  : `Aplicar ${toWrite.length} ${toWrite.length === 1 ? 'objeto' : 'objetos'}`}
              </Button>
            </Group>
          </Group>
        </Stack>
      )}
      {jobId && current && current.status !== 'running' && (
        <Group justify="flex-end" mt="md">
          <Button onClick={onClose}>Fechar</Button>
        </Group>
      )}
    </Modal>
  )
}
