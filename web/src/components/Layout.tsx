import { NavLink as RouterNavLink, Outlet, useLocation, useNavigate } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import {
  ActionIcon,
  AppShell,
  Avatar,
  Badge,
  Burger,
  Group,
  Indicator,
  Kbd,
  Menu,
  ScrollArea,
  Text,
  Tooltip,
  UnstyledButton,
  useComputedColorScheme,
  useMantineColorScheme,
} from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { Spotlight, spotlight, type SpotlightActionData } from '@mantine/spotlight'
import {
  IconCertificate2,
  IconChevronRight,
  IconGauge,
  IconHistory,
  IconKey,
  IconLogout,
  IconMoon,
  IconPlus,
  IconSearch,
  IconSun,
  IconTransfer,
  IconUserShield,
  type Icon,
} from '@tabler/icons-react'
import { api, canWrite, isAdmin } from '../api'
import { useAuth } from '../auth'
import { RESOURCES } from '../resources'
import { Logo } from './Logo'
import { initials, ROLE_COLOR, ROLE_LABEL } from './roles'
import classes from './Layout.module.css'

interface Status {
  reachable: boolean
  server?: string
  latency_ms: number
}

/** Shared with the overview page: one request feeds both. */
export function useGatewayStatus() {
  return useQuery({ queryKey: ['status'], queryFn: () => api<Status>('GET', '/api/status'), refetchInterval: 30_000 })
}

function GatewayIndicator() {
  const { data, isPending } = useGatewayStatus()
  if (isPending) return null
  const ok = !!data?.reachable
  return (
    <Tooltip label={ok ? `Admin API conectada · ${data?.latency_ms} ms` : 'Admin API indisponível'}>
      <Badge
        variant="light"
        color={ok ? 'teal' : 'red'}
        leftSection={
          <Indicator color={ok ? 'teal' : 'red'} size={7} processing={ok} position="middle-center" children={null} />
        }
        visibleFrom="sm"
        styles={{ section: { width: 12, display: 'flex', justifyContent: 'center' } }}
      >
        {ok ? data?.server?.replace('/', ' ') || 'online' : 'offline'}
      </Badge>
    </Tooltip>
  )
}

export function Layout() {
  const { me, logout } = useAuth()
  const [opened, { toggle, close }] = useDisclosure()
  const { setColorScheme } = useMantineColorScheme()
  const scheme = useComputedColorScheme('light')
  const location = useLocation()
  const navigate = useNavigate()

  const isActive = (to: string) => (to === '/' ? location.pathname === '/' : location.pathname.startsWith(to))
  const link = (to: string, label: string, IconCmp: Icon) => (
    <RouterNavLink key={to} to={to} className={classes.link} data-active={isActive(to) || undefined} onClick={close}>
      <IconCmp className={classes.linkIcon} size={19} stroke={1.6} />
      <span>{label}</span>
    </RouterNavLink>
  )

  const actions: SpotlightActionData[] = [
    {
      id: 'home',
      label: 'Visão geral',
      description: 'Status do gateway e resumo',
      onClick: () => void navigate('/'),
      leftSection: <IconGauge size={20} stroke={1.5} />,
    },
    ...RESOURCES.map((r) => ({
      id: `go-${r.key}`,
      label: r.label,
      description: r.description,
      keywords: [r.key, r.singular],
      onClick: () => navigate(`/r/${r.key}`),
      leftSection: <r.icon size={20} stroke={1.5} />,
    })),
    {
      id: 'acme',
      label: 'Certificados automáticos',
      description: "Let's Encrypt via DNS (Cloudflare)",
      keywords: ['acme', 'letsencrypt', 'renovar', 'tls'],
      onClick: () => void navigate('/certificados'),
      leftSection: <IconCertificate2 size={20} stroke={1.5} />,
    },
    ...(canWrite(me.role)
      ? [
          {
            id: 'kong',
            label: 'Migração do Kong',
            description: 'Trazer services, rotas e consumers do Kong',
            keywords: ['kong', 'migrar', 'importar'],
            onClick: () => navigate('/migracao/kong'),
            leftSection: <IconTransfer size={20} stroke={1.5} />,
          },
        ]
      : []),
    ...(canWrite(me.role)
      ? RESOURCES.slice(0, 6).map((r) => ({
          id: `new-${r.key}`,
          label: `Novo ${r.singular}`,
          description: `Criar ${r.singular}`,
          keywords: ['criar', 'novo', 'adicionar', r.key],
          onClick: () => navigate(`/r/${r.key}?novo=1`),
          leftSection: <IconPlus size={20} stroke={1.5} />,
        }))
      : []),
    ...(isAdmin(me.role)
      ? [
          {
            id: 'users',
            label: 'Usuários',
            description: 'Acessos à dashboard',
            onClick: () => navigate('/usuarios'),
            leftSection: <IconUserShield size={20} stroke={1.5} />,
          },
          {
            id: 'audit',
            label: 'Auditoria',
            description: 'Trilha de eventos',
            onClick: () => navigate('/auditoria'),
            leftSection: <IconHistory size={20} stroke={1.5} />,
          },
        ]
      : []),
    {
      id: 'password',
      label: 'Alterar senha',
      onClick: () => void navigate('/conta'),
      leftSection: <IconKey size={20} stroke={1.5} />,
    },
    {
      id: 'theme',
      label: scheme === 'dark' ? 'Tema claro' : 'Tema escuro',
      onClick: () => setColorScheme(scheme === 'dark' ? 'light' : 'dark'),
      leftSection: scheme === 'dark' ? <IconSun size={20} stroke={1.5} /> : <IconMoon size={20} stroke={1.5} />,
    },
  ]

  return (
    <AppShell
      header={{ height: 60 }}
      navbar={{ width: 260, breakpoint: 'sm', collapsed: { mobile: !opened } }}
      padding="lg"
    >
      <AppShell.Header className={classes.header}>
        <Group h="100%" px="md" justify="space-between" wrap="nowrap">
          <Group gap="sm" wrap="nowrap">
            <Burger opened={opened} onClick={toggle} hiddenFrom="sm" size="sm" />
            <Logo />
          </Group>
          <Group gap="sm" wrap="nowrap">
            <UnstyledButton className={classes.search} onClick={() => spotlight.open()}>
              <Group justify="space-between" wrap="nowrap" h="100%">
                <Group gap={8} wrap="nowrap">
                  <IconSearch size={15} stroke={1.8} />
                  <Text size="sm" visibleFrom="sm">
                    Buscar...
                  </Text>
                </Group>
                <Kbd size="xs" visibleFrom="sm">
                  Ctrl + K
                </Kbd>
              </Group>
            </UnstyledButton>
            <GatewayIndicator />
            <Tooltip label={scheme === 'dark' ? 'Tema claro' : 'Tema escuro'}>
              <ActionIcon
                variant="default"
                size={34}
                onClick={() => setColorScheme(scheme === 'dark' ? 'light' : 'dark')}
                aria-label="Alternar tema"
              >
                {scheme === 'dark' ? <IconSun size={18} stroke={1.6} /> : <IconMoon size={18} stroke={1.6} />}
              </ActionIcon>
            </Tooltip>
          </Group>
        </Group>
      </AppShell.Header>

      <AppShell.Navbar p="sm" className={classes.navbar}>
        <AppShell.Section grow component={ScrollArea} scrollbarSize={6}>
          {link('/', 'Visão geral', IconGauge)}
          <div className={classes.groupLabel}>Gateway</div>
          {RESOURCES.flatMap((r) => [
            link(`/r/${r.key}`, r.label, r.icon),
            ...(r.key === 'ssls' ? [link('/certificados', 'Certificados automáticos', IconCertificate2)] : []),
          ])}
          {canWrite(me.role) && (
            <>
              <div className={classes.groupLabel}>Migração</div>
              {link('/migracao/kong', 'Kong → APISIX', IconTransfer)}
            </>
          )}
          {isAdmin(me.role) && (
            <>
              <div className={classes.groupLabel}>Administração</div>
              {link('/usuarios', 'Usuários', IconUserShield)}
              {link('/auditoria', 'Auditoria', IconHistory)}
            </>
          )}
        </AppShell.Section>

        <AppShell.Section className={classes.footer}>
          <Menu position="top-start" width={230} withArrow>
            <Menu.Target>
              <UnstyledButton className={classes.user}>
                <Group wrap="nowrap" gap="sm">
                  <Avatar radius="xl" color={ROLE_COLOR[me.role]} variant="filled">
                    {initials(me.username)}
                  </Avatar>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <Text size="sm" fw={600} truncate>
                      {me.username}
                    </Text>
                    <Text size="xs" c="dimmed">
                      {ROLE_LABEL[me.role]}
                    </Text>
                  </div>
                  <IconChevronRight size={14} stroke={1.5} />
                </Group>
              </UnstyledButton>
            </Menu.Target>
            <Menu.Dropdown>
              <Menu.Label>Versão {me.version}</Menu.Label>
              <Menu.Item component={RouterNavLink} to="/conta" leftSection={<IconKey size={16} />}>
                Alterar senha
              </Menu.Item>
              <Menu.Divider />
              <Menu.Item color="red" leftSection={<IconLogout size={16} />} onClick={logout}>
                Sair
              </Menu.Item>
            </Menu.Dropdown>
          </Menu>
        </AppShell.Section>
      </AppShell.Navbar>

      <AppShell.Main className={classes.main}>
        <div style={{ maxWidth: 1400, margin: '0 auto' }}>
          <Outlet />
        </div>
      </AppShell.Main>

      <Spotlight
        actions={actions}
        shortcut={['mod + K', '/']}
        nothingFound="Nada encontrado"
        highlightQuery
        limit={12}
        searchProps={{ leftSection: <IconSearch size={18} stroke={1.5} />, placeholder: 'Ir para... ou criar...' }}
      />
    </AppShell>
  )
}
