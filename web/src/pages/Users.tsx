import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ActionIcon,
  Alert,
  Avatar,
  Card,
  Badge,
  Button,
  Group,
  Menu,
  Modal,
  PasswordInput,
  Select,
  Stack,
  Table,
  Text,
  TextInput,
} from '@mantine/core'
import { modals } from '@mantine/modals'
import { notifications } from '@mantine/notifications'
import {
  IconDots,
  IconKey,
  IconLockOpen,
  IconPlus,
  IconShieldCheck,
  IconShieldOff,
  IconTrash,
  IconUserCheck,
  IconUserOff,
  IconUserShield,
} from '@tabler/icons-react'
import { PageHeader } from '../components/PageHeader'
import { avatarColor, initials } from '../components/roles'
import { api, type DashboardUser, type Role } from '../api'
import { useAuth } from '../auth'

const isRole = (r: string): r is Role => r === 'viewer' || r === 'operator' || r === 'admin'

const confirm = (title: string, text: string, run: () => void, danger = false) =>
  modals.openConfirmModal({
    title,
    children: <Text size="sm">{text}</Text>,
    labels: { confirm: 'Confirmar', cancel: 'Cancelar' },
    confirmProps: danger ? { color: 'red' } : undefined,
    onConfirm: run,
  })

const ROLES = [
  { value: 'viewer', label: 'Leitura (viewer)' },
  { value: 'operator', label: 'Operador (operator)' },
  { value: 'admin', label: 'Administrador (admin)' },
]

const fmt = (s: string | null) => (s ? new Date(s).toLocaleString('pt-BR') : '—')

export function Users() {
  const { me } = useAuth()
  const qc = useQueryClient()
  const { data: users = [], isPending } = useQuery({
    queryKey: ['users'],
    queryFn: () => api<DashboardUser[]>('GET', '/api/users'),
  })
  const [creating, setCreating] = useState(false)
  const [resetting, setResetting] = useState<DashboardUser | null>(null)

  const action = useMutation({
    mutationFn: ({ method, path, body }: { method: string; path: string; body?: unknown }) => api(method, path, body),
    onSuccess: () => {
      notifications.show({ color: 'green', message: 'Alteração aplicada' })
      void qc.invalidateQueries({ queryKey: ['users'] })
    },
    onError: (e) => notifications.show({ color: 'red', message: e.message }),
  })

  return (
    <>
      <PageHeader
        icon={IconUserShield}
        title="Usuários"
        description="Quem acessa a dashboard e com qual perfil. MFA é obrigatório para todos."
        actions={
          <Button leftSection={<IconPlus size={16} />} onClick={() => setCreating(true)}>
            Novo usuário
          </Button>
        }
      />

      <Card padding={0}>
        <Table.ScrollContainer minWidth={760}>
          <Table verticalSpacing="sm" horizontalSpacing="md" highlightOnHover>
            <Table.Thead bg="var(--mantine-color-default-hover)">
              <Table.Tr>
                <Table.Th>Usuário</Table.Th>
                <Table.Th>Perfil</Table.Th>
                <Table.Th>Status</Table.Th>
                <Table.Th>MFA</Table.Th>
                <Table.Th>Último login</Table.Th>
                <Table.Th w={60} />
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {isPending && (
                <Table.Tr>
                  <Table.Td colSpan={6}>Carregando...</Table.Td>
                </Table.Tr>
              )}
              {users.map((u) => {
                const self = u.username === me.username
                return (
                  <Table.Tr key={u.id}>
                    <Table.Td>
                      <Group gap="sm" wrap="nowrap">
                        <Avatar radius="xl" color={avatarColor(u.username)}>
                          {initials(u.username)}
                        </Avatar>
                        <div>
                          <Group gap={6}>
                            <Text size="sm" fw={600}>
                              {u.username}
                            </Text>
                            {self && (
                              <Badge size="xs" variant="outline">
                                você
                              </Badge>
                            )}
                          </Group>
                          <Text size="xs" c="dimmed">
                            desde {new Date(u.created_at).toLocaleDateString('pt-BR')}
                          </Text>
                        </div>
                      </Group>
                    </Table.Td>
                    <Table.Td>
                      <Select
                        size="xs"
                        w={200}
                        data={ROLES}
                        value={u.role}
                        disabled={self}
                        allowDeselect={false}
                        onChange={(role) =>
                          role &&
                          isRole(role) &&
                          confirm(
                            'Alterar perfil',
                            `Alterar o perfil de ${u.username} para ${role}? As sessões ativas do usuário serão encerradas.`,
                            () =>
                              action.mutate({
                                method: 'PATCH',
                                path: `/api/users/${u.id}`,
                                body: { role },
                              }),
                          )
                        }
                      />
                    </Table.Td>
                    <Table.Td>
                      <Group gap={4}>
                        {u.disabled ? (
                          <Badge color="gray" variant="light">
                            inativo
                          </Badge>
                        ) : (
                          <Badge color="teal" variant="dot">
                            ativo
                          </Badge>
                        )}
                        {u.locked && (
                          <Badge color="red" variant="light">
                            bloqueado
                          </Badge>
                        )}
                        {u.must_change_password && (
                          <Badge color="yellow" variant="light">
                            trocar senha
                          </Badge>
                        )}
                      </Group>
                    </Table.Td>
                    <Table.Td>
                      {u.mfa_enrolled ? (
                        <Badge color="teal" variant="light" leftSection={<IconShieldCheck size={12} />}>
                          configurado
                        </Badge>
                      ) : (
                        <Badge color="orange" variant="light">
                          pendente
                        </Badge>
                      )}
                    </Table.Td>
                    <Table.Td>
                      <Text size="sm">{fmt(u.last_login_at)}</Text>
                    </Table.Td>
                    <Table.Td>
                      {!self && (
                        <Menu position="bottom-end">
                          <Menu.Target>
                            <ActionIcon variant="subtle" color="gray">
                              <IconDots size={18} />
                            </ActionIcon>
                          </Menu.Target>
                          <Menu.Dropdown>
                            <Menu.Item leftSection={<IconKey size={16} />} onClick={() => setResetting(u)}>
                              Definir senha temporária
                            </Menu.Item>
                            <Menu.Item
                              leftSection={<IconShieldOff size={16} />}
                              onClick={() =>
                                confirm(
                                  'Redefinir MFA',
                                  `O usuário ${u.username} deverá configurar o MFA novamente no próximo login.`,
                                  () => action.mutate({ method: 'POST', path: `/api/users/${u.id}/reset-mfa` }),
                                )
                              }
                            >
                              Redefinir MFA
                            </Menu.Item>
                            {u.locked && (
                              <Menu.Item
                                leftSection={<IconLockOpen size={16} />}
                                onClick={() => action.mutate({ method: 'POST', path: `/api/users/${u.id}/unlock` })}
                              >
                                Desbloquear
                              </Menu.Item>
                            )}
                            <Menu.Item
                              leftSection={u.disabled ? <IconUserCheck size={16} /> : <IconUserOff size={16} />}
                              onClick={() =>
                                action.mutate({
                                  method: 'PATCH',
                                  path: `/api/users/${u.id}`,
                                  body: { disabled: !u.disabled },
                                })
                              }
                            >
                              {u.disabled ? 'Reativar' : 'Desativar'}
                            </Menu.Item>
                            <Menu.Divider />
                            <Menu.Item
                              color="red"
                              leftSection={<IconTrash size={16} />}
                              onClick={() =>
                                confirm(
                                  'Excluir usuário',
                                  `Excluir ${u.username}? Prefira desativar para manter o histórico claro.`,
                                  () => action.mutate({ method: 'DELETE', path: `/api/users/${u.id}` }),
                                  true,
                                )
                              }
                            >
                              Excluir
                            </Menu.Item>
                          </Menu.Dropdown>
                        </Menu>
                      )}
                    </Table.Td>
                  </Table.Tr>
                )
              })}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
      </Card>

      <CreateUserModal opened={creating} onClose={() => setCreating(false)} />
      <ResetPasswordModal user={resetting} onClose={() => setResetting(null)} />
    </>
  )
}

function CreateUserModal({ opened, onClose }: { opened: boolean; onClose: () => void }) {
  const qc = useQueryClient()
  const [username, setUsername] = useState('')
  const [role, setRole] = useState<string>('viewer')
  const [password, setPassword] = useState('')
  const create = useMutation({
    mutationFn: () => api('POST', '/api/users', { username, role, password }),
    onSuccess: () => {
      notifications.show({ color: 'green', message: `Usuário ${username} criado` })
      void qc.invalidateQueries({ queryKey: ['users'] })
      setUsername('')
      setPassword('')
      onClose()
    },
  })
  return (
    <Modal opened={opened} onClose={onClose} title="Novo usuário">
      <Stack>
        {create.error && <Alert color="red">{create.error.message}</Alert>}
        <TextInput label="Usuário" value={username} onChange={(e) => setUsername(e.currentTarget.value)} required />
        <Select label="Perfil" data={ROLES} value={role} onChange={(v) => v && setRole(v)} allowDeselect={false} />
        <PasswordInput
          label="Senha temporária"
          description="O usuário deverá trocá-la e configurar o MFA no primeiro acesso. Compartilhe por um canal seguro."
          value={password}
          onChange={(e) => setPassword(e.currentTarget.value)}
          required
        />
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            Cancelar
          </Button>
          <Button onClick={() => create.mutate()} loading={create.isPending}>
            Criar
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}

function ResetPasswordModal({ user, onClose }: { user: DashboardUser | null; onClose: () => void }) {
  const qc = useQueryClient()
  const [password, setPassword] = useState('')
  const reset = useMutation({
    mutationFn: () => {
      if (!user) throw new Error('Nenhum usuário selecionado')
      return api('POST', `/api/users/${user.id}/password`, { password })
    },
    onSuccess: () => {
      notifications.show({ color: 'green', message: 'Senha temporária definida' })
      void qc.invalidateQueries({ queryKey: ['users'] })
      setPassword('')
      onClose()
    },
  })
  return (
    <Modal opened={!!user} onClose={onClose} title={`Senha temporária para ${user?.username ?? ''}`}>
      <Stack>
        {reset.error && <Alert color="red">{reset.error.message}</Alert>}
        <PasswordInput
          label="Nova senha temporária"
          description="As sessões do usuário serão encerradas e ele deverá trocar a senha no próximo login."
          value={password}
          onChange={(e) => setPassword(e.currentTarget.value)}
        />
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            Cancelar
          </Button>
          <Button onClick={() => reset.mutate()} loading={reset.isPending}>
            Definir
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}
