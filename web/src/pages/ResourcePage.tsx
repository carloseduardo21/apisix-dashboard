import { useEffect, useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ActionIcon,
  Alert,
  Badge,
  Button,
  Card,
  Code,
  CopyButton,
  Drawer,
  Group,
  Menu,
  Pagination,
  ScrollArea,
  Select,
  Skeleton,
  Stack,
  Table,
  Text,
  TextInput,
  ThemeIcon,
  Tooltip,
} from '@mantine/core'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import {
  IconAlertCircle,
  IconCertificate2,
  IconCheck,
  IconCopy,
  IconDots,
  IconEdit,
  IconEye,
  IconKey,
  IconPlus,
  IconRefresh,
  IconSearch,
  IconTrash,
} from '@tabler/icons-react'
import { ConsumerCredentials } from '../components/ConsumerCredentials'
import { api, canWrite, isStreamDisabled, itemId, type AdminItem, type AdminList } from '../api'
import { useAuth } from '../auth'
import { EmptyState } from '../components/EmptyState'
import { PageHeader } from '../components/PageHeader'
import { ResourceEditor } from '../components/ResourceEditor'
import { ServiceDetails } from '../components/ServiceDetails'
import { isManagedSSL, type ResourceDef } from '../resources'

// Fields managed by APISIX that must not be sent back on update.
const READ_ONLY = ['create_time', 'update_time', 'validity_start', 'validity_end']

function editable(value: Record<string, unknown>) {
  const out = { ...value }
  for (const k of READ_ONLY) delete out[k]
  return out
}

const fmtTime = (v: unknown) => (typeof v === 'number' ? new Date(v * 1000).toLocaleString('pt-BR') : '—')

type Editing = { mode: 'create' } | { mode: 'edit'; id: string; value: Record<string, unknown> }

export function ResourcePage({ def }: { def: ResourceDef }) {
  const { me } = useAuth()
  const qc = useQueryClient()
  const writable = canWrite(me.role)
  const [params, setParams] = useSearchParams()
  const [filter, setFilter] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState('25')
  const [viewing, setViewing] = useState<AdminItem | null>(null)
  const [editing, setEditing] = useState<Editing | null>(null)

  // "?novo=1" (from the command palette) opens the creation form.
  useEffect(() => {
    if (params.get('novo') && writable) {
      setEditing({ mode: 'create' })
      setParams({}, { replace: true })
    }
  }, [params, setParams, writable])

  const queryKey = ['apisix', def.key]
  const { data, isPending, error, refetch, isFetching } = useQuery({
    queryKey,
    queryFn: () => api<AdminList>('GET', `/api/apisix/${def.key}`),
  })

  // "?editar=<id>" (e.g. from a service's routes) opens that item's editor.
  useEffect(() => {
    const id = params.get('editar')
    if (!id || !data) return
    const item = data.list.find((i) => itemId(def.key, i) === id)
    if (item) {
      if (writable && !isManagedSSL(def.key, item.value)) setEditing({ mode: 'edit', id, value: editable(item.value) })
      else setViewing(item)
    }
    setParams({}, { replace: true })
  }, [params, setParams, data, def.key, writable])

  const rows = useMemo(() => {
    const list = data?.list ?? []
    const q = filter.trim().toLowerCase()
    return list
      .map((item) => ({ item, id: itemId(def.key, item) }))
      .filter(({ item, id }) => !q || def.search(item.value, id).toLowerCase().includes(q))
  }, [data, filter, def])

  const size = Number(pageSize)
  const pages = Math.max(1, Math.ceil(rows.length / size))
  const current = Math.min(page, pages)
  const visible = rows.slice((current - 1) * size, current * size)

  const save = useMutation({
    mutationFn: async ({ value, id }: { value: Record<string, unknown>; id: string }) => {
      if (editing?.mode === 'edit') {
        const path = def.create === 'put-body' ? `/api/apisix/${def.key}` : `/api/apisix/${def.key}/${editing.id}`
        return api('PUT', path, value)
      }
      if (def.create === 'post') {
        return id ? api('PUT', `/api/apisix/${def.key}/${id}`, value) : api('POST', `/api/apisix/${def.key}`, value)
      }
      if (def.create === 'put-body') return api('PUT', `/api/apisix/${def.key}`, value)
      return api('PUT', `/api/apisix/${def.key}/${id}`, value)
    },
    onSuccess: () => {
      notifications.show({ color: 'teal', icon: <IconCheck size={18} />, title: 'Salvo', message: `${def.singular} salvo com sucesso` })
      qc.invalidateQueries({ queryKey })
    },
  })

  const remove = useMutation({
    mutationFn: (id: string) => api('DELETE', `/api/apisix/${def.key}/${id}`),
    onSuccess: () => {
      notifications.show({ color: 'teal', icon: <IconCheck size={18} />, title: 'Excluído', message: `${def.singular} excluído` })
      qc.invalidateQueries({ queryKey })
    },
    onError: (e) => notifications.show({ color: 'red', title: 'Falha ao excluir', message: (e as Error).message }),
  })

  const confirmDelete = (id: string) =>
    modals.openConfirmModal({
      title: <Text fw={600}>Excluir {def.singular}</Text>,
      children: (
        <Text size="sm">
          Tem certeza que deseja excluir <Code>{id}</Code>? A alteração é aplicada imediatamente no gateway e fica registrada na auditoria.
        </Text>
      ),
      labels: { confirm: 'Excluir', cancel: 'Cancelar' },
      confirmProps: { color: 'red', leftSection: <IconTrash size={16} /> },
      onConfirm: () => remove.mutate(id),
    })

  const openEdit = (item: AdminItem) => setEditing({ mode: 'edit', id: itemId(def.key, item), value: editable(item.value) })
  // A click opens the editor; services open their summary (with routes), and
  // read-only users or dashboard-managed SSLs get the read-only view.
  const openRow = (item: AdminItem) => {
    if (def.key !== 'services' && writable && !isManagedSSL(def.key, item.value)) openEdit(item)
    else setViewing(item)
  }
  const total = data?.total ?? data?.list?.length

  return (
    <>
      <PageHeader
        icon={def.icon}
        title={def.label}
        description={def.description}
        badge={
          total !== undefined && (
            <Badge variant="light" size="lg" radius="xl">
              {total}
            </Badge>
          )
        }
        actions={
          writable && (
            <Button leftSection={<IconPlus size={16} />} onClick={() => setEditing({ mode: 'create' })}>
              Novo {def.singular}
            </Button>
          )
        }
      />

      {isStreamDisabled(error) ? (
        <Alert color="gray" icon={<IconAlertCircle size={16} />} title="Modo stream desativado" mb="md">
          Habilite <code>apisix.proxy_mode: http&stream</code> e <code>stream_proxy</code> no config.yaml do APISIX para usar stream routes.
        </Alert>
      ) : (
        error && (
          <Alert color="red" icon={<IconAlertCircle size={16} />} title="Erro ao carregar" mb="md">
            {(error as Error).message}
          </Alert>
        )
      )}

      <Card padding={0}>
        <Group justify="space-between" p="md" gap="sm">
          <TextInput
            placeholder={`Buscar ${def.label.toLowerCase()}...`}
            leftSection={<IconSearch size={16} />}
            value={filter}
            onChange={(e) => {
              setFilter(e.currentTarget.value)
              setPage(1)
            }}
            w={320}
          />
          <Tooltip label="Recarregar">
            <ActionIcon variant="default" size={36} onClick={() => refetch()} loading={isFetching} aria-label="Recarregar">
              <IconRefresh size={18} />
            </ActionIcon>
          </Tooltip>
        </Group>

        {isPending ? (
          <Stack p="md" gap="sm">
            {[0, 1, 2, 3].map((i) => (
              <Skeleton key={i} h={36} />
            ))}
          </Stack>
        ) : rows.length === 0 ? (
          <EmptyState
            icon={def.icon}
            title={filter ? 'Nenhum resultado' : `Nenhum ${def.singular} cadastrado`}
            description={filter ? 'Tente outro termo de busca.' : def.description}
            action={
              writable &&
              !filter &&
              !error && (
                <Button variant="light" leftSection={<IconPlus size={16} />} onClick={() => setEditing({ mode: 'create' })}>
                  Criar {def.singular}
                </Button>
              )
            }
          />
        ) : (
          <>
            <Table.ScrollContainer minWidth={800}>
              <Table highlightOnHover verticalSpacing="sm" horizontalSpacing="md">
                <Table.Thead bg="var(--mantine-color-default-hover)">
                  <Table.Tr>
                    {def.columns.map((c) => (
                      <Table.Th key={c.label}>{c.label}</Table.Th>
                    ))}
                    <Table.Th w={60} />
                  </Table.Tr>
                </Table.Thead>
                <Table.Tbody>
                  {visible.map(({ item, id }) => (
                    <Table.Tr key={item.key} style={{ cursor: 'pointer' }} onClick={() => openRow(item)}>
                      {def.columns.map((c) => (
                        <Table.Td key={c.label}>{c.render(item.value, id)}</Table.Td>
                      ))}
                      <Table.Td onClick={(e) => e.stopPropagation()}>
                        <Menu position="bottom-end" withinPortal shadow="md" width={180}>
                          <Menu.Target>
                            <ActionIcon variant="subtle" color="gray" aria-label="Ações">
                              <IconDots size={18} />
                            </ActionIcon>
                          </Menu.Target>
                          <Menu.Dropdown>
                            <Menu.Item leftSection={<IconEye size={16} />} onClick={() => setViewing(item)}>
                              Visualizar
                            </Menu.Item>
                            {def.key === 'consumers' && (
                              <Menu.Item leftSection={<IconKey size={16} />} onClick={() => setViewing(item)}>
                                Credenciais
                              </Menu.Item>
                            )}
                            {isManagedSSL(def.key, item.value) ? (
                              <Menu.Item component={Link} to="/certificados" leftSection={<IconCertificate2 size={16} />}>
                                Certificados automáticos
                              </Menu.Item>
                            ) : writable && (
                              <>
                                <Menu.Item leftSection={<IconEdit size={16} />} onClick={() => openEdit(item)}>
                                  Editar
                                </Menu.Item>
                                <Menu.Divider />
                                <Menu.Item color="red" leftSection={<IconTrash size={16} />} onClick={() => confirmDelete(id)}>
                                  Excluir
                                </Menu.Item>
                              </>
                            )}
                          </Menu.Dropdown>
                        </Menu>
                      </Table.Td>
                    </Table.Tr>
                  ))}
                </Table.Tbody>
              </Table>
            </Table.ScrollContainer>
            <Group justify="space-between" p="md" gap="sm">
              <Text size="sm" c="dimmed">
                {(current - 1) * size + 1}–{Math.min(current * size, rows.length)} de {rows.length}
              </Text>
              <Group gap="sm">
                <Select
                  size="xs"
                  w={110}
                  data={['10', '25', '50', '100'].map((v) => ({ value: v, label: `${v} / página` }))}
                  value={pageSize}
                  onChange={(v) => {
                    setPageSize(v ?? '25')
                    setPage(1)
                  }}
                  allowDeselect={false}
                />
                {pages > 1 && <Pagination size="sm" total={pages} value={current} onChange={setPage} />}
              </Group>
            </Group>
          </>
        )}
      </Card>

      <Drawer
        opened={!!viewing}
        onClose={() => setViewing(null)}
        position="right"
        size="xl"
        title={
          viewing && (
            <Group gap="sm">
              <ThemeIcon variant="light" size={34}>
                <def.icon size={18} />
              </ThemeIcon>
              <div>
                <Text fw={600}>{String(viewing.value.name ?? viewing.value.username ?? def.singular)}</Text>
                <Text size="xs" c="dimmed" ff="monospace">
                  {itemId(def.key, viewing)}
                </Text>
              </div>
            </Group>
          )
        }
        scrollAreaComponent={ScrollArea.Autosize}
      >
        {viewing && (
          <Stack>
            <Group justify="space-between">
              <Text size="xs" c="dimmed">
                Criado em {fmtTime(viewing.value.create_time)} · atualizado em {fmtTime(viewing.value.update_time)}
              </Text>
              <Group gap="xs">
                <CopyButton value={JSON.stringify(viewing.value, null, 2)}>
                  {({ copied, copy }) => (
                    <Button size="xs" variant="default" leftSection={copied ? <IconCheck size={14} /> : <IconCopy size={14} />} onClick={copy}>
                      {copied ? 'Copiado' : 'Copiar JSON'}
                    </Button>
                  )}
                </CopyButton>
                {isManagedSSL(def.key, viewing.value) ? (
                  <Button size="xs" variant="light" color="orange" component={Link} to="/certificados" leftSection={<IconCertificate2 size={14} />}>
                    Emitido automaticamente
                  </Button>
                ) : writable && (
                  <Button
                    size="xs"
                    leftSection={<IconEdit size={14} />}
                    onClick={() => {
                      openEdit(viewing)
                      setViewing(null)
                    }}
                  >
                    Editar
                  </Button>
                )}
              </Group>
            </Group>
            {def.key === 'consumers' && typeof viewing.value.username === 'string' && (
              <Card withBorder>
                <ConsumerCredentials username={viewing.value.username} writable={writable} />
              </Card>
            )}
            {def.key === 'services' ? (
              <ServiceDetails id={itemId(def.key, viewing)} value={viewing.value} />
            ) : (
              <Code block>{JSON.stringify(viewing.value, null, 2)}</Code>
            )}
          </Stack>
        )}
      </Drawer>

      <ResourceEditor
        def={def}
        opened={!!editing}
        editing={editing?.mode === 'edit' ? editing : undefined}
        onClose={() => setEditing(null)}
        onSubmit={async (value, id) => {
          await save.mutateAsync({ value, id })
        }}
      />
    </>
  )
}
