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

// Credentials live under the consumer: /consumers/{username}/credentials/{id}.
const CREDENTIAL_DEF: ResourceDef = {
  key: 'credentials',
  label: 'Credenciais',
  singular: 'credencial',
  feminine: true,
  description: 'Chave, usuário/senha ou segredo usado pelo consumer para se autenticar',
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

const fmtTime = (v: unknown) => (typeof v === 'number' ? new Date(v * 1000).toLocaleString('pt-BR') : '—')

/** APISIX before 3.10 has no credentials endpoint. */
const unsupported = (e: unknown) => e instanceof ApiError && e.status === 404

export function ConsumerCredentials({ username, writable }: { username: string; writable: boolean }) {
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
    onSuccess: () => done('Excluída', 'Credencial excluída'),
    onError: (e) => notifications.show({ color: 'red', title: 'Falha ao excluir', message: (e).message }),
  })

  const confirmDelete = (id: string) =>
    modals.openConfirmModal({
      title: <Text fw={600}>Excluir credencial</Text>,
      children: (
        <Text size="sm">
          Excluir <Code>{id}</Code> de <Code>{username}</Code>? Clientes que usam essa credencial deixam de se autenticar imediatamente.
        </Text>
      ),
      labels: { confirm: 'Excluir', cancel: 'Cancelar' },
      confirmProps: { color: 'red', leftSection: <IconTrash size={16} /> },
      onConfirm: () => remove.mutate(id),
    })

  if (unsupported(error)) {
    return (
      <Alert color="gray" icon={<IconAlertCircle size={16} />}>
        Este APISIX não suporta credenciais (disponível a partir da versão 3.10). Configure a autenticação nos plugins do consumer.
      </Alert>
    )
  }

  const list = data?.list ?? []

  return (
    <Stack gap="sm">
      <Group justify="space-between">
        <div>
          <Text fw={600}>Credenciais</Text>
          <Text size="xs" c="dimmed">
            Um consumer pode ter várias credenciais, por exemplo uma por aplicação, para trocar uma sem derrubar as outras.
          </Text>
        </div>
        {writable && (
          <Button size="xs" leftSection={<IconPlus size={14} />} onClick={() => setEditing({ mode: 'create' })}>
            Nova credencial
          </Button>
        )}
      </Group>

      {error && (
        <Alert color="red" icon={<IconAlertCircle size={16} />}>
          {(error).message}
        </Alert>
      )}

      {isPending ? (
        <Skeleton h={72} />
      ) : list.length === 0 ? (
        !error && (
          <Text size="sm" c="dimmed" ta="center" py="md">
            Nenhuma credencial cadastrada
          </Text>
        )
      ) : (
        <Table.ScrollContainer minWidth={520}>
          <Table verticalSpacing="xs" withTableBorder>
            <Table.Thead>
              <Table.Tr>
                <Table.Th>ID</Table.Th>
                <Table.Th>Autenticação</Table.Th>
                <Table.Th>Descrição</Table.Th>
                <Table.Th>Atualizada</Table.Th>
                {writable && <Table.Th w={72} />}
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {list.map((item) => {
                const id = credentialId(item)
                const plugins = Object.keys((item.value.plugins as Obj) ?? {})
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
                          <Text size="sm" c="dimmed">—</Text>
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
                          <Tooltip label="Editar">
                            <ActionIcon variant="subtle" color="gray" aria-label={`Editar ${id}`} onClick={() => setEditing({ mode: 'edit', id, value: editable(item.value) })}>
                              <IconEdit size={16} />
                            </ActionIcon>
                          </Tooltip>
                          <Tooltip label="Excluir">
                            <ActionIcon variant="subtle" color="red" aria-label={`Excluir ${id}`} onClick={() => confirmDelete(id)}>
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
          if (!Object.keys((value.plugins as Obj) ?? {}).length) throw new Error('Adicione um plugin de autenticação')
          await api('PUT', `${base}/${encodeURIComponent(id)}`, value)
          done('Salva', `Credencial ${id} salva`)
        }}
      />
    </Stack>
  )
}
