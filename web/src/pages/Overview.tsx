import { useQueries, useQuery } from '@tanstack/react-query'
import { Link } from 'react-router'
import {
  Avatar,
  Badge,
  Card,
  Grid,
  Group,
  Loader,
  Progress,
  SimpleGrid,
  Skeleton,
  Stack,
  Text,
  ThemeIcon,
  Timeline,
  Title,
  UnstyledButton,
} from '@mantine/core'
import {
  IconAlertTriangle,
  IconArrowUpRight,
  IconCertificate,
  IconCircleCheck,
  IconHistory,
  IconPlugConnectedX,
} from '@tabler/icons-react'
import { api, isAdmin, isStreamDisabled, itemId, type AdminList, type AuditEvent } from '../api'
import { useAuth } from '../auth'
import { useGatewayStatus } from '../components/Layout'
import { PageHeader } from '../components/PageHeader'
import { avatarColor, initials } from '../components/roles'
import { actionLabel, actionColor } from './Audit'
import { MAIN_RESOURCES, RESOURCE_BY_KEY } from '../resources'
import classes from './Overview.module.css'

type Obj = Record<string, unknown>

const COLORS: Record<string, string> = {
  routes: 'indigo',
  services: 'violet',
  upstreams: 'cyan',
  consumers: 'teal',
  ssls: 'orange',
  plugin_configs: 'grape',
  global_rules: 'pink',
  stream_routes: 'blue',
}

/** Second line of each stat card, derived from the listed objects. */
function detail(key: string, values: Obj[]): string {
  switch (key) {
    case 'routes': {
      const off = values.filter((v) => v.status === 0).length
      return off ? `${values.length - off} ativas · ${off} inativas` : 'todas ativas'
    }
    case 'upstreams': {
      const checked = values.filter((v) => v.checks).length
      return `${checked} com health check`
    }
    case 'consumers': {
      const auth = values.filter((v) => Object.keys((v.plugins as Obj) ?? {}).some((p) => p.includes('auth'))).length
      return `${auth} com autenticação`
    }
    case 'ssls': {
      const soon = values.filter(
        (v) => typeof v.validity_end === 'number' && v.validity_end * 1000 - Date.now() < 30 * 86_400_000,
      ).length
      return soon ? `${soon} vencem em até 30 dias` : 'nenhum vencendo'
    }
    case 'global_rules':
      return 'aplicadas a todo o tráfego'
    default: {
      const plugins = new Set(values.flatMap((v) => Object.keys((v.plugins as Obj) ?? {})))
      return `${plugins.size} plugin(s) distintos`
    }
  }
}

const daysLeft = (end: number) => Math.floor((end * 1000 - Date.now()) / 86_400_000)

function GatewayCard() {
  const { data, isPending, error } = useGatewayStatus()
  const ok = !!data?.reachable
  return (
    <Card className={ok ? classes.statusOk : classes.statusDown} padding="lg">
      {isPending ? (
        <Skeleton h={44} />
      ) : (
        <Group justify="space-between" wrap="nowrap">
          <Group wrap="nowrap">
            <ThemeIcon size={48} radius="md" color={ok ? 'teal' : 'red'} variant="light">
              {ok ? <IconCircleCheck size={28} stroke={1.6} /> : <IconPlugConnectedX size={28} stroke={1.6} />}
            </ThemeIcon>
            <div>
              <Text fw={700} size="lg">
                {ok ? 'Gateway operacional' : 'Admin API indisponível'}
              </Text>
              <Text size="sm" c="dimmed">
                {ok
                  ? `${data?.server || 'APISIX'} · Admin API respondendo em ${data?.latency_ms} ms`
                  : ((data as { error?: string })?.error ?? (error as Error)?.message)}
              </Text>
            </div>
          </Group>
          <Badge size="lg" variant="dot" color={ok ? 'teal' : 'red'} visibleFrom="sm">
            {ok ? 'online' : 'offline'}
          </Badge>
        </Group>
      )}
    </Card>
  )
}

export function Overview() {
  const { me } = useAuth()
  const lists = useQueries({
    queries: MAIN_RESOURCES.map((key) => ({
      queryKey: ['apisix', key],
      queryFn: () => api<AdminList>('GET', `/api/apisix/${key}`),
    })),
  })
  const audit = useQuery({
    queryKey: ['audit', 'recent'],
    queryFn: () => api<{ items: AuditEvent[] }>('GET', '/api/audit?limit=8'),
    enabled: isAdmin(me.role),
  })

  const ssl = lists[MAIN_RESOURCES.indexOf('ssls')]
  const certs = (ssl.data?.list ?? [])
    .filter((i) => typeof i.value.validity_end === 'number')
    .map((i) => ({
      id: itemId('ssls', i),
      snis: ([] as unknown[]).concat(i.value.snis ?? i.value.sni ?? []).join(', '),
      days: daysLeft(i.value.validity_end as number),
    }))
    .sort((a, b) => a.days - b.days)

  const hour = new Date().getHours()
  const greeting = hour < 12 ? 'Bom dia' : hour < 18 ? 'Boa tarde' : 'Boa noite'

  return (
    <Stack gap="lg">
      <PageHeader title={`${greeting}, ${me.username}`} description="Resumo da configuração do seu gateway APISIX" />

      <GatewayCard />

      <SimpleGrid cols={{ base: 2, lg: 4 }} spacing={{ base: 'sm', sm: 'md' }}>
        {MAIN_RESOURCES.map((key, i) => {
          const def = RESOURCE_BY_KEY[key]
          const q = lists[i]
          const values = (q.data?.list ?? []).map((it) => it.value)
          const disabled = isStreamDisabled(q.error)
          return (
            <UnstyledButton key={key} component={Link} to={`/r/${key}`} className={classes.statLink}>
              <Card className={classes.stat} h="100%">
                <Group justify="space-between" align="flex-start">
                  <Text className={classes.statTitle}>{def.label}</Text>
                  <ThemeIcon variant="light" color={COLORS[key]} size={36} radius="md">
                    <def.icon size={20} stroke={1.6} />
                  </ThemeIcon>
                </Group>
                {q.isPending ? (
                  <Skeleton h={34} w={60} mt="sm" />
                ) : disabled ? (
                  <Text className={classes.statValue} c="dimmed">
                    —
                  </Text>
                ) : q.error ? (
                  <Text className={classes.statValue} c="red">
                    !
                  </Text>
                ) : (
                  <Text className={classes.statValue}>{q.data?.total ?? values.length}</Text>
                )}
                <Group justify="space-between" mt={4}>
                  <Text size="xs" c="dimmed">
                    {disabled
                      ? 'modo stream desativado'
                      : q.error
                        ? 'erro ao carregar'
                        : q.data
                          ? detail(key, values)
                          : ' '}
                  </Text>
                  <IconArrowUpRight size={14} className={classes.statArrow} />
                </Group>
              </Card>
            </UnstyledButton>
          )
        })}
      </SimpleGrid>

      <Grid gap="md">
        {isAdmin(me.role) && (
          <Grid.Col span={{ base: 12, md: 7 }}>
            <Card h="100%">
              <Group justify="space-between" mb="md">
                <Group gap="xs">
                  <IconHistory size={20} stroke={1.6} />
                  <Title order={4}>Atividade recente</Title>
                </Group>
                <Text component={Link} to="/auditoria" size="sm" c="indigo" fw={500}>
                  Ver auditoria
                </Text>
              </Group>
              {audit.isPending ? (
                <Loader size="sm" />
              ) : (
                <Timeline bulletSize={30} lineWidth={2}>
                  {(audit.data?.items ?? []).map((e) => (
                    <Timeline.Item
                      key={e.id}
                      bullet={
                        <Avatar size={26} radius="xl" color={avatarColor(e.actor)} variant="filled">
                          {initials(e.actor.replace(/^cli:/, ''))}
                        </Avatar>
                      }
                    >
                      <Group gap={6} wrap="nowrap">
                        <Text size="sm" fw={600}>
                          {e.actor}
                        </Text>
                        <Badge
                          size="xs"
                          variant="light"
                          color={e.outcome === 'failure' ? 'red' : actionColor(e.action)}
                        >
                          {actionLabel(e.action)}
                        </Badge>
                      </Group>
                      <Text size="xs" c="dimmed" mt={2}>
                        {e.resource ? `${e.resource}${e.resource_id ? ` / ${e.resource_id}` : ''} · ` : ''}
                        {new Date(e.ts).toLocaleString('pt-BR')}
                      </Text>
                    </Timeline.Item>
                  ))}
                </Timeline>
              )}
            </Card>
          </Grid.Col>
        )}
        <Grid.Col span={{ base: 12, md: isAdmin(me.role) ? 5 : 12 }}>
          <Card h="100%">
            <Group justify="space-between" mb="md">
              <Group gap="xs">
                <IconCertificate size={20} stroke={1.6} />
                <Title order={4}>Validade dos certificados</Title>
              </Group>
              <Text component={Link} to="/r/ssls" size="sm" c="indigo" fw={500}>
                Ver todos
              </Text>
            </Group>
            {ssl.isPending ? (
              <Loader size="sm" />
            ) : certs.length === 0 ? (
              <Text size="sm" c="dimmed">
                Nenhum certificado com data de validade cadastrado.
              </Text>
            ) : (
              <Stack gap="md">
                {certs.slice(0, 6).map((c) => {
                  const color = c.days < 0 ? 'red' : c.days < 30 ? 'orange' : 'teal'
                  return (
                    <div key={c.id}>
                      <Group justify="space-between" mb={4} wrap="nowrap">
                        <Text size="sm" fw={500} truncate>
                          {c.snis || c.id}
                        </Text>
                        <Group gap={4} wrap="nowrap">
                          {c.days < 30 && <IconAlertTriangle size={14} color={`var(--mantine-color-${color}-6)`} />}
                          <Text size="xs" c={color} fw={600}>
                            {c.days < 0 ? 'vencido' : `${c.days} dias`}
                          </Text>
                        </Group>
                      </Group>
                      <Progress
                        value={Math.max(2, Math.min(100, (c.days / 365) * 100))}
                        color={color}
                        size="sm"
                        radius="xl"
                      />
                    </div>
                  )
                })}
              </Stack>
            )}
          </Card>
        </Grid.Col>
      </Grid>
    </Stack>
  )
}
