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
import { avatarColor, initials, ROLE_LABEL } from '../components/roles'
import { api, type DashboardUser, type Role } from '../api'
import { useAuth } from '../auth'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'
import i18n from '../i18n'
import { formatDate, formatDateTime } from '../i18n/format'

const isRole = (r: string): r is Role => r === 'viewer' || r === 'operator' || r === 'admin'

const confirm = (title: string, text: string, run: () => void, danger = false) =>
  modals.openConfirmModal({
    title,
    children: <Text size="sm">{text}</Text>,
    labels: { confirm: i18n.t('common.confirm'), cancel: i18n.t('common.cancel') },
    confirmProps: danger ? { color: 'red' } : undefined,
    onConfirm: run,
  })

const ROLE_KEYS: Role[] = ['viewer', 'operator', 'admin']

/** Select options for the roles, in the current language. */
const roleOptions = (t: TFunction) => ROLE_KEYS.map((r) => ({ value: r, label: `${t(ROLE_LABEL[r])} (${r})` }))

const fmt = (s: string | null) => (s ? formatDateTime(s) : '—')

export function Users() {
  const { t } = useTranslation()
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
      notifications.show({ color: 'green', message: t('users.applied') })
      void qc.invalidateQueries({ queryKey: ['users'] })
    },
    onError: (e) => notifications.show({ color: 'red', message: e.message }),
  })

  return (
    <>
      <PageHeader
        icon={IconUserShield}
        title={t('users.title')}
        description={t('users.description')}
        actions={
          <Button leftSection={<IconPlus size={16} />} onClick={() => setCreating(true)}>
            {t('users.new')}
          </Button>
        }
      />

      <Card padding={0}>
        <Table.ScrollContainer minWidth={760}>
          <Table verticalSpacing="sm" horizontalSpacing="md" highlightOnHover>
            <Table.Thead bg="var(--mantine-color-default-hover)">
              <Table.Tr>
                <Table.Th>{t('users.col_user')}</Table.Th>
                <Table.Th>{t('users.col_role')}</Table.Th>
                <Table.Th>{t('common.status')}</Table.Th>
                <Table.Th>MFA</Table.Th>
                <Table.Th>{t('users.col_last_login')}</Table.Th>
                <Table.Th w={60} />
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {isPending && (
                <Table.Tr>
                  <Table.Td colSpan={6}>{t('users.loading')}</Table.Td>
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
                                {t('users.you')}
                              </Badge>
                            )}
                          </Group>
                          <Text size="xs" c="dimmed">
                            {t('users.since', { date: formatDate(u.created_at) })}
                          </Text>
                        </div>
                      </Group>
                    </Table.Td>
                    <Table.Td>
                      <Select
                        size="xs"
                        w={200}
                        data={roleOptions(t)}
                        value={u.role}
                        disabled={self}
                        allowDeselect={false}
                        onChange={(role) =>
                          role &&
                          isRole(role) &&
                          confirm(
                            t('users.change_role_title'),
                            t('users.change_role_confirm', { username: u.username, role: t(ROLE_LABEL[role]) }),
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
                            {t('common.inactive')}
                          </Badge>
                        ) : (
                          <Badge color="teal" variant="dot">
                            {t('common.active')}
                          </Badge>
                        )}
                        {u.locked && (
                          <Badge color="red" variant="light">
                            {t('users.locked')}
                          </Badge>
                        )}
                        {u.must_change_password && (
                          <Badge color="yellow" variant="light">
                            {t('users.must_change')}
                          </Badge>
                        )}
                      </Group>
                    </Table.Td>
                    <Table.Td>
                      {u.mfa_enrolled ? (
                        <Badge color="teal" variant="light" leftSection={<IconShieldCheck size={12} />}>
                          {t('users.mfa_enrolled')}
                        </Badge>
                      ) : (
                        <Badge color="orange" variant="light">
                          {t('users.mfa_pending')}
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
                            <ActionIcon variant="subtle" color="gray" aria-label={t('page.actions')}>
                              <IconDots size={18} />
                            </ActionIcon>
                          </Menu.Target>
                          <Menu.Dropdown>
                            <Menu.Item leftSection={<IconKey size={16} />} onClick={() => setResetting(u)}>
                              {t('users.set_temp_password')}
                            </Menu.Item>
                            <Menu.Item
                              leftSection={<IconShieldOff size={16} />}
                              onClick={() =>
                                confirm(
                                  t('users.reset_mfa'),
                                  t('users.reset_mfa_confirm', { username: u.username }),
                                  () => action.mutate({ method: 'POST', path: `/api/users/${u.id}/reset-mfa` }),
                                )
                              }
                            >
                              {t('users.reset_mfa')}
                            </Menu.Item>
                            {u.locked && (
                              <Menu.Item
                                leftSection={<IconLockOpen size={16} />}
                                onClick={() => action.mutate({ method: 'POST', path: `/api/users/${u.id}/unlock` })}
                              >
                                {t('users.unlock')}
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
                              {u.disabled ? t('users.enable') : t('users.disable')}
                            </Menu.Item>
                            <Menu.Divider />
                            <Menu.Item
                              color="red"
                              leftSection={<IconTrash size={16} />}
                              onClick={() =>
                                confirm(
                                  t('users.delete_title'),
                                  t('users.delete_confirm', { username: u.username }),
                                  () => action.mutate({ method: 'DELETE', path: `/api/users/${u.id}` }),
                                  true,
                                )
                              }
                            >
                              {t('common.delete')}
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
  const { t } = useTranslation()
  const qc = useQueryClient()
  const [username, setUsername] = useState('')
  const [role, setRole] = useState<string>('viewer')
  const [password, setPassword] = useState('')
  const create = useMutation({
    mutationFn: () => api('POST', '/api/users', { username, role, password }),
    onSuccess: () => {
      notifications.show({ color: 'green', message: t('users.created', { username }) })
      void qc.invalidateQueries({ queryKey: ['users'] })
      setUsername('')
      setPassword('')
      onClose()
    },
  })
  return (
    <Modal opened={opened} onClose={onClose} title={t('users.new')}>
      <Stack>
        {create.error && <Alert color="red">{create.error.message}</Alert>}
        <TextInput
          label={t('users.col_user')}
          value={username}
          onChange={(e) => setUsername(e.currentTarget.value)}
          required
        />
        <Select
          label={t('users.col_role')}
          data={roleOptions(t)}
          value={role}
          onChange={(v) => v && setRole(v)}
          allowDeselect={false}
        />
        <PasswordInput
          label={t('users.temp_password')}
          description={t('users.temp_password_hint')}
          value={password}
          onChange={(e) => setPassword(e.currentTarget.value)}
          required
        />
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button onClick={() => create.mutate()} loading={create.isPending}>
            {t('users.create')}
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}

function ResetPasswordModal({ user, onClose }: { user: DashboardUser | null; onClose: () => void }) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const [password, setPassword] = useState('')
  const reset = useMutation({
    mutationFn: () => {
      if (!user) throw new Error(t('users.no_user_selected'))
      return api('POST', `/api/users/${user.id}/password`, { password })
    },
    onSuccess: () => {
      notifications.show({ color: 'green', message: t('users.temp_password_set') })
      void qc.invalidateQueries({ queryKey: ['users'] })
      setPassword('')
      onClose()
    },
  })
  return (
    <Modal opened={!!user} onClose={onClose} title={t('users.temp_password_for', { username: user?.username ?? '' })}>
      <Stack>
        {reset.error && <Alert color="red">{reset.error.message}</Alert>}
        <PasswordInput
          label={t('users.new_temp_password')}
          description={t('users.new_temp_password_hint')}
          value={password}
          onChange={(e) => setPassword(e.currentTarget.value)}
        />
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button onClick={() => reset.mutate()} loading={reset.isPending}>
            {t('users.set')}
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}
