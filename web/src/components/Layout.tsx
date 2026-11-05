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
import { useTranslation } from 'react-i18next'
import {
  IconCertificate2,
  IconChevronRight,
  IconGauge,
  IconHistory,
  IconKey,
  IconLanguage,
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
import { LANGUAGES, setLanguage } from '../i18n'
import { RESOURCES } from '../resources'
import { Logo } from './Logo'
import { initials, ROLE_COLOR, ROLE_LABEL } from './roles'
import classes from './Layout.module.css'

interface Status {
  reachable: boolean
  server?: string
  latency_ms: number
  // Set when the Admin API is unreachable (English text + translatable code).
  error?: string
  code?: string
  params?: Record<string, string>
}

/** Shared with the overview page: one request feeds both. */
export function useGatewayStatus() {
  return useQuery({ queryKey: ['status'], queryFn: () => api<Status>('GET', '/api/status'), refetchInterval: 30_000 })
}

function GatewayIndicator() {
  const { t } = useTranslation()
  const { data, isPending } = useGatewayStatus()
  if (isPending) return null
  const ok = !!data?.reachable
  return (
    <Tooltip label={ok ? t('nav.gateway_online', { latency: data?.latency_ms }) : t('nav.gateway_offline')}>
      <Badge
        variant="light"
        color={ok ? 'teal' : 'red'}
        leftSection={
          <Indicator color={ok ? 'teal' : 'red'} size={7} processing={ok} position="middle-center">
            {null}
          </Indicator>
        }
        visibleFrom="sm"
        styles={{ section: { width: 12, display: 'flex', justifyContent: 'center' } }}
      >
        {ok ? data?.server?.replace('/', ' ') || t('overview.online') : t('overview.offline')}
      </Badge>
    </Tooltip>
  )
}

export function Layout() {
  const { t, i18n } = useTranslation()
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
      label: t('nav.overview'),
      description: t('nav.overview_desc'),
      onClick: () => void navigate('/'),
      leftSection: <IconGauge size={20} stroke={1.5} />,
    },
    ...RESOURCES.map((r) => ({
      id: `go-${r.key}`,
      label: t(`resources.${r.key}.label`),
      description: t(`resources.${r.key}.description`),
      keywords: [r.key, t(`resources.${r.key}.singular`)],
      onClick: () => navigate(`/r/${r.key}`),
      leftSection: <r.icon size={20} stroke={1.5} />,
    })),
    {
      id: 'acme',
      label: t('nav.acme'),
      description: t('nav.acme_desc'),
      keywords: t('nav.keywords_acme').split(' '),
      onClick: () => void navigate('/certificados'),
      leftSection: <IconCertificate2 size={20} stroke={1.5} />,
    },
    ...(canWrite(me.role)
      ? [
          {
            id: 'kong',
            label: t('nav.kong_migration'),
            description: t('nav.kong_desc'),
            keywords: t('nav.keywords_kong').split(' '),
            onClick: () => navigate('/migracao/kong'),
            leftSection: <IconTransfer size={20} stroke={1.5} />,
          },
        ]
      : []),
    ...(canWrite(me.role)
      ? RESOURCES.slice(0, 6).map((r) => ({
          id: `new-${r.key}`,
          label: t('resources.new', {
            context: t(`resources.${r.key}.gender`),
            name: t(`resources.${r.key}.singular`),
          }),
          description: t('resources.create', { name: t(`resources.${r.key}.singular`) }),
          keywords: [...t('nav.keywords_new').split(' '), r.key],
          onClick: () => navigate(`/r/${r.key}?novo=1`),
          leftSection: <IconPlus size={20} stroke={1.5} />,
        }))
      : []),
    ...(isAdmin(me.role)
      ? [
          {
            id: 'users',
            label: t('nav.users'),
            description: t('nav.users_desc'),
            onClick: () => navigate('/usuarios'),
            leftSection: <IconUserShield size={20} stroke={1.5} />,
          },
          {
            id: 'audit',
            label: t('nav.audit'),
            description: t('nav.audit_desc'),
            onClick: () => navigate('/auditoria'),
            leftSection: <IconHistory size={20} stroke={1.5} />,
          },
        ]
      : []),
    {
      id: 'password',
      label: t('nav.change_password'),
      onClick: () => void navigate('/conta'),
      leftSection: <IconKey size={20} stroke={1.5} />,
    },
    {
      id: 'theme',
      label: scheme === 'dark' ? t('nav.theme_light') : t('nav.theme_dark'),
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
                    {t('nav.search')}
                  </Text>
                </Group>
                <Kbd size="xs" visibleFrom="sm">
                  Ctrl + K
                </Kbd>
              </Group>
            </UnstyledButton>
            <GatewayIndicator />
            <Tooltip label={scheme === 'dark' ? t('nav.theme_light') : t('nav.theme_dark')}>
              <ActionIcon
                variant="default"
                size={34}
                onClick={() => setColorScheme(scheme === 'dark' ? 'light' : 'dark')}
                aria-label={t('nav.toggle_theme')}
              >
                {scheme === 'dark' ? <IconSun size={18} stroke={1.6} /> : <IconMoon size={18} stroke={1.6} />}
              </ActionIcon>
            </Tooltip>
          </Group>
        </Group>
      </AppShell.Header>

      <AppShell.Navbar p="sm" className={classes.navbar}>
        <AppShell.Section grow component={ScrollArea} scrollbarSize={6}>
          {link('/', t('nav.overview'), IconGauge)}
          <div className={classes.groupLabel}>{t('nav.gateway')}</div>
          {RESOURCES.flatMap((r) =>
            r.key === 'ssls'
              ? [
                  link(`/r/${r.key}`, t(`resources.${r.key}.label`), r.icon),
                  link('/certificados', t('nav.acme'), IconCertificate2),
                ]
              : [link(`/r/${r.key}`, t(`resources.${r.key}.label`), r.icon)],
          )}
          {canWrite(me.role) && (
            <>
              <div className={classes.groupLabel}>{t('nav.migration')}</div>
              {link('/migracao/kong', t('nav.kong'), IconTransfer)}
            </>
          )}
          {isAdmin(me.role) && (
            <>
              <div className={classes.groupLabel}>{t('nav.admin')}</div>
              {link('/usuarios', t('nav.users'), IconUserShield)}
              {link('/auditoria', t('nav.audit'), IconHistory)}
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
                      {t(ROLE_LABEL[me.role])}
                    </Text>
                  </div>
                  <IconChevronRight size={14} stroke={1.5} />
                </Group>
              </UnstyledButton>
            </Menu.Target>
            <Menu.Dropdown>
              <Menu.Label>{t('common.version', { version: me.version })}</Menu.Label>
              <Menu.Item component={RouterNavLink} to="/conta" leftSection={<IconKey size={16} />}>
                {t('nav.change_password')}
              </Menu.Item>
              <Menu.Divider />
              <Menu.Label>
                <IconLanguage size={12} style={{ verticalAlign: -1, marginRight: 4 }} />
                {t('common.language')}
              </Menu.Label>
              {LANGUAGES.map((l) => (
                <Menu.Item
                  key={l.code}
                  onClick={() => setLanguage(l.code)}
                  fw={i18n.language === l.code ? 700 : undefined}
                  aria-current={i18n.language === l.code || undefined}
                >
                  {l.label}
                </Menu.Item>
              ))}
              <Menu.Divider />
              <Menu.Item color="red" leftSection={<IconLogout size={16} />} onClick={logout}>
                {t('nav.logout')}
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
        nothingFound={t('nav.nothing_found')}
        highlightQuery
        limit={12}
        searchProps={{ leftSection: <IconSearch size={18} stroke={1.5} />, placeholder: t('nav.search_placeholder') }}
      />
    </AppShell>
  )
}
