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
import { text } from '../values'
import { Trans, useTranslation } from 'react-i18next'
import { formatDateTime } from '../i18n/format'

// Fields managed by APISIX that must not be sent back on update.
const READ_ONLY = ['create_time', 'update_time', 'validity_start', 'validity_end']

function editable(value: Record<string, unknown>) {
  const out = { ...value }
  for (const k of READ_ONLY) delete out[k]
  return out
}

const fmtTime = (v: unknown) => (typeof v === 'number' ? formatDateTime(v * 1000) : '—')

type Editing = { mode: 'create' } | { mode: 'edit'; id: string; value: Record<string, unknown> }

export function ResourcePage({ def }: { def: ResourceDef }) {
  const { t } = useTranslation()
  const name = t(`resources.${def.key}.singular`)
  const context = t(`resources.${def.key}.gender`)
  const label = t(`resources.${def.key}.label`)
  const description = t(`resources.${def.key}.description`)
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
      // oxlint-disable-next-line react/set-state-in-effect -- consumes a one-shot URL command and clears it in the router (external system)
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
      // oxlint-disable-next-line react/set-state-in-effect -- consumes a one-shot URL command and clears it in the router (external system)
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
    // oxlint-disable-next-line react/no-unstable-nested-components -- mutation callback that builds a notification icon element, not a component
    onSuccess: () => {
      notifications.show({
        color: 'teal',
        icon: <IconCheck size={18} />,
        title: t('page.saved'),
        message: t('resources.saved', { name, context }),
      })
      void qc.invalidateQueries({ queryKey })
    },
  })

  const remove = useMutation({
    mutationFn: (id: string) => api('DELETE', `/api/apisix/${def.key}/${id}`),
    // oxlint-disable-next-line react/no-unstable-nested-components -- mutation callback that builds a notification icon element, not a component
    onSuccess: () => {
      notifications.show({
        color: 'teal',
        icon: <IconCheck size={18} />,
        title: t('page.deleted'),
        message: t('resources.deleted', { name, context }),
      })
      void qc.invalidateQueries({ queryKey })
    },
    onError: (e) => notifications.show({ color: 'red', title: t('page.delete_failed'), message: e.message }),
  })

  const confirmDelete = (id: string) =>
    modals.openConfirmModal({
      title: <Text fw={600}>{t('resources.delete_title', { name })}</Text>,
      children: (
        <Text size="sm">
          <Trans i18nKey="page.delete_confirm" values={{ id }} components={{ code: <Code /> }} />
        </Text>
      ),
      labels: { confirm: t('common.delete'), cancel: t('common.cancel') },
      confirmProps: { color: 'red', leftSection: <IconTrash size={16} /> },
      onConfirm: () => remove.mutate(id),
    })

  const openEdit = (item: AdminItem) =>
    setEditing({ mode: 'edit', id: itemId(def.key, item), value: editable(item.value) })
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
        title={label}
        description={description}
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
              {t('resources.new', { name, context })}
            </Button>
          )
        }
      />

      {isStreamDisabled(error) ? (
        <Alert color="gray" icon={<IconAlertCircle size={16} />} title={t('page.stream_disabled_title')} mb="md">
          <Trans i18nKey="page.stream_disabled" components={{ code: <code /> }} />
        </Alert>
      ) : (
        error && (
          <Alert color="red" icon={<IconAlertCircle size={16} />} title={t('common.load_error')} mb="md">
            {error.message}
          </Alert>
        )
      )}

      <Card padding={0}>
        <Group justify="space-between" p="md" gap="sm">
          <TextInput
            placeholder={t('resources.search', { label: label.toLowerCase() })}
            leftSection={<IconSearch size={16} />}
            value={filter}
            onChange={(e) => {
              setFilter(e.currentTarget.value)
              setPage(1)
            }}
            w={320}
          />
          <Tooltip label={t('common.reload')}>
            <ActionIcon
              variant="default"
              size={36}
              onClick={() => refetch()}
              loading={isFetching}
              aria-label={t('common.reload')}
            >
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
            title={filter ? t('resources.no_results') : t('resources.empty', { name, context })}
            description={filter ? t('resources.try_other') : description}
            action={
              writable &&
              !filter &&
              !error && (
                <Button
                  variant="light"
                  leftSection={<IconPlus size={16} />}
                  onClick={() => setEditing({ mode: 'create' })}
                >
                  {t('resources.create', { name })}
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
                      <Table.Th key={c.label}>{t(c.label)}</Table.Th>
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
                            <ActionIcon variant="subtle" color="gray" aria-label={t('page.actions')}>
                              <IconDots size={18} />
                            </ActionIcon>
                          </Menu.Target>
                          <Menu.Dropdown>
                            <Menu.Item leftSection={<IconEye size={16} />} onClick={() => setViewing(item)}>
                              {t('page.view')}
                            </Menu.Item>
                            {def.key === 'consumers' && (
                              <Menu.Item leftSection={<IconKey size={16} />} onClick={() => setViewing(item)}>
                                {t('page.credentials')}
                              </Menu.Item>
                            )}
                            {isManagedSSL(def.key, item.value) ? (
                              <Menu.Item
                                component={Link}
                                to="/certificados"
                                leftSection={<IconCertificate2 size={16} />}
                              >
                                {t('nav.acme')}
                              </Menu.Item>
                            ) : (
                              writable && (
                                <>
                                  <Menu.Item leftSection={<IconEdit size={16} />} onClick={() => openEdit(item)}>
                                    {t('common.edit')}
                                  </Menu.Item>
                                  <Menu.Divider />
                                  <Menu.Item
                                    color="red"
                                    leftSection={<IconTrash size={16} />}
                                    onClick={() => confirmDelete(id)}
                                  >
                                    {t('common.delete')}
                                  </Menu.Item>
                                </>
                              )
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
                {t('page.range', {
                  from: (current - 1) * size + 1,
                  to: Math.min(current * size, rows.length),
                  total: rows.length,
                })}
              </Text>
              <Group gap="sm">
                <Select
                  size="xs"
                  w={110}
                  data={['10', '25', '50', '100'].map((v) => ({ value: v, label: t('page.per_page', { n: v }) }))}
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
                <Text fw={600}>{text(viewing.value.name ?? viewing.value.username ?? name)}</Text>
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
                {t('page.timestamps', {
                  created: fmtTime(viewing.value.create_time),
                  updated: fmtTime(viewing.value.update_time),
                })}
              </Text>
              <Group gap="xs">
                <CopyButton value={JSON.stringify(viewing.value, null, 2)}>
                  {({ copied, copy }) => (
                    <Button
                      size="xs"
                      variant="default"
                      leftSection={copied ? <IconCheck size={14} /> : <IconCopy size={14} />}
                      onClick={copy}
                    >
                      {copied ? t('page.copied') : t('page.copy_json')}
                    </Button>
                  )}
                </CopyButton>
                {isManagedSSL(def.key, viewing.value) ? (
                  <Button
                    size="xs"
                    variant="light"
                    color="orange"
                    component={Link}
                    to="/certificados"
                    leftSection={<IconCertificate2 size={14} />}
                  >
                    {t('page.issued_automatically')}
                  </Button>
                ) : (
                  writable && (
                    <Button
                      size="xs"
                      leftSection={<IconEdit size={14} />}
                      onClick={() => {
                        openEdit(viewing)
                        setViewing(null)
                      }}
                    >
                      {t('common.edit')}
                    </Button>
                  )
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
