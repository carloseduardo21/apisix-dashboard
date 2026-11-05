import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ActionIcon, Alert, Badge, Button, Code, Group, Skeleton, Stack, Table, Text, Tooltip } from '@mantine/core'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import { IconAlertCircle, IconCheck, IconEdit, IconKey, IconPlus, IconTrash } from '@tabler/icons-react'
import { api, ApiError, type AdminItem, type AdminList } from '../api'
import type { Obj } from '../schema/types'
import type { ResourceDef } from '../resources'
import { ResourceEditor } from './ResourceEditor'
import { asObj } from '../values'
import { Trans, useTranslation } from 'react-i18next'
import { formatDateTime } from '../i18n/format'

// Credentials live under the consumer: /consumers/{username}/credentials/{id}.
const CREDENTIAL_DEF: ResourceDef = {
  key: 'credentials',
  icon: IconKey,
  columns: [],
  create: 'put-id',
  template: { plugins: {} },
  search: (_v, id) => id,
}

const READ_ONLY = ['id', 'create_time', 'update_time']

function editable(value: Obj) {
  const out = { ...value }
  for (const k of READ_ONLY) delete out[k]
  return out
}

const credentialId = (item: AdminItem) => item.key.split('/').pop() ?? ''

const fmtTime = (v: unknown) => (typeof v === 'number' ? formatDateTime(v * 1000) : '—')

/** APISIX before 3.10 has no credentials endpoint. */
const unsupported = (e: unknown) => e instanceof ApiError && e.status === 404

export function ConsumerCredentials({ username, writable }: { username: string; writable: boolean }) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const base = `/api/apisix/consumers/${encodeURIComponent(username)}/credentials`
  const queryKey = ['apisix', 'consumers', username, 'credentials']
  const [editing, setEditing] = useState<{ mode: 'create' } | { mode: 'edit'; id: string; value: Obj } | null>(null)

  const { data, isPending, error } = useQuery({
    queryKey,
    queryFn: () => api<AdminList>('GET', base),
    retry: (n, e) => !unsupported(e) && n < 2,
  })

  const done = (title: string, message: string) => {
    notifications.show({ color: 'teal', icon: <IconCheck size={18} />, title, message })
    void qc.invalidateQueries({ queryKey })
  }

  const remove = useMutation({
    mutationFn: (id: string) => api('DELETE', `${base}/${encodeURIComponent(id)}`),
    onSuccess: () => done(t('credentials.deleted_title'), t('credentials.deleted')),
    onError: (e) => notifications.show({ color: 'red', title: t('page.delete_failed'), message: e.message }),
  })

  const confirmDelete = (id: string) =>
    modals.openConfirmModal({
      title: <Text fw={600}>{t('credentials.delete_title')}</Text>,
      children: (
        <Text size="sm">
          <Trans i18nKey="credentials.delete_confirm" values={{ id, username }} components={{ code: <Code /> }} />
        </Text>
      ),
      labels: { confirm: t('common.delete'), cancel: t('common.cancel') },
      confirmProps: { color: 'red', leftSection: <IconTrash size={16} /> },
      onConfirm: () => remove.mutate(id),
    })

  if (unsupported(error)) {
    return (
      <Alert color="gray" icon={<IconAlertCircle size={16} />}>
        {t('credentials.unsupported')}
      </Alert>
    )
  }

  const list = data?.list ?? []

  return (
    <Stack gap="sm">
      <Group justify="space-between">
        <div>
          <Text fw={600}>{t('credentials.title')}</Text>
          <Text size="xs" c="dimmed">
            {t('credentials.help')}
          </Text>
        </div>
        {writable && (
          <Button size="xs" leftSection={<IconPlus size={14} />} onClick={() => setEditing({ mode: 'create' })}>
            {t('credentials.new')}
          </Button>
        )}
      </Group>

      {error && (
        <Alert color="red" icon={<IconAlertCircle size={16} />}>
          {error.message}
        </Alert>
      )}

      {isPending ? (
        <Skeleton h={72} />
      ) : list.length === 0 ? (
        !error && (
          <Text size="sm" c="dimmed" ta="center" py="md">
            {t('credentials.empty')}
          </Text>
        )
      ) : (
        <Table.ScrollContainer minWidth={520}>
          <Table verticalSpacing="xs" withTableBorder>
            <Table.Thead>
              <Table.Tr>
                <Table.Th>ID</Table.Th>
                <Table.Th>{t('credentials.col_auth')}</Table.Th>
                <Table.Th>{t('common.description')}</Table.Th>
                <Table.Th>{t('credentials.col_updated')}</Table.Th>
                {writable && <Table.Th w={72} />}
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {list.map((item) => {
                const id = credentialId(item)
                const plugins = Object.keys(asObj(item.value.plugins) ?? {})
                return (
                  <Table.Tr key={item.key}>
                    <Table.Td>
                      <Text size="sm" ff="monospace">
                        {id}
                      </Text>
                    </Table.Td>
                    <Table.Td>
                      <Group gap={4}>
                        {plugins.length ? (
                          plugins.map((n) => (
                            <Badge key={n} size="sm" variant="light">
                              {n}
                            </Badge>
                          ))
                        ) : (
                          <Text size="sm" c="dimmed">
                            —
                          </Text>
                        )}
                      </Group>
                    </Table.Td>
                    <Table.Td>
                      <Text size="sm" c="dimmed" lineClamp={1}>
                        {typeof item.value.desc === 'string' && item.value.desc ? item.value.desc : '—'}
                      </Text>
                    </Table.Td>
                    <Table.Td>
                      <Text size="xs" c="dimmed">
                        {fmtTime(item.value.update_time)}
                      </Text>
                    </Table.Td>
                    {writable && (
                      <Table.Td>
                        <Group gap={2} wrap="nowrap">
                          <Tooltip label={t('common.edit')}>
                            <ActionIcon
                              variant="subtle"
                              color="gray"
                              aria-label={t('credentials.edit_aria', { id })}
                              onClick={() => setEditing({ mode: 'edit', id, value: editable(item.value) })}
                            >
                              <IconEdit size={16} />
                            </ActionIcon>
                          </Tooltip>
                          <Tooltip label={t('common.delete')}>
                            <ActionIcon
                              variant="subtle"
                              color="red"
                              aria-label={t('credentials.delete_aria', { id })}
                              onClick={() => confirmDelete(id)}
                            >
                              <IconTrash size={16} />
                            </ActionIcon>
                          </Tooltip>
                        </Group>
                      </Table.Td>
                    )}
                  </Table.Tr>
                )
              })}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
      )}

      <ResourceEditor
        def={CREDENTIAL_DEF}
        opened={!!editing}
        editing={editing?.mode === 'edit' ? editing : undefined}
        onClose={() => setEditing(null)}
        onSubmit={async (value, id) => {
          if (!Object.keys(asObj(value.plugins) ?? {}).length) throw new Error(t('credentials.plugin_required'))
          await api('PUT', `${base}/${encodeURIComponent(id)}`, value)
          done(t('credentials.saved_title'), t('credentials.saved', { id }))
        }}
      />
    </Stack>
  )
}
