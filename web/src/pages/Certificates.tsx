import { useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ActionIcon,
  Alert,
  Anchor,
  Badge,
  Button,
  Card,
  Checkbox,
  Code,
  Group,
  Loader,
  Menu,
  Modal,
  Progress,
  SegmentedControl,
  Select,
  SimpleGrid,
  Skeleton,
  Stack,
  TagsInput,
  Text,
  TextInput,
  ThemeIcon,
  Timeline,
  Tooltip,
} from '@mantine/core'
import { notifications } from '@mantine/notifications'
import {
  IconAlertCircle,
  IconAlertTriangle,
  IconCertificate2,
  IconCheck,
  IconCircleCheck,
  IconClockHour4,
  IconCloud,
  IconDots,
  IconInfoCircle,
  IconListDetails,
  IconPlus,
  IconRefresh,
  IconTrash,
  IconX,
} from '@tabler/icons-react'
import { api, isAdmin, itemId, type AdminList } from '../api'
import { useAuth } from '../auth'
import { EmptyState } from '../components/EmptyState'
import { PageHeader } from '../components/PageHeader'

interface LogEntry {
  ts: string
  level: 'info' | 'warn' | 'error'
  message: string
}

export interface AcmeCert {
  id: number
  domains: string[]
  ssl_id: string
  environment: 'production' | 'staging'
  key_type: 'ec256' | 'rsa2048'
  status: 'pending' | 'issuing' | 'valid' | 'error'
  last_error?: string
  log: LogEntry[]
  not_before: string | null
  not_after: string | null
  renew_at: string | null
  fingerprint?: string
  issuer?: string
  failures: number
  next_attempt_at: string | null
  issued_at: string | null
  created_by: string
}

interface AcmeInfo {
  enabled: boolean
  email?: string
  renew_before_days?: number
  certificates: AcmeCert[]
}

interface Provider {
  token_ok: boolean
  error?: string
  zones: { name: string; status: string }[]
  terms_of_service?: string
}

const fmtDate = (v: string | null) => (v ? new Date(v).toLocaleDateString('pt-BR') : '—')
const fmtDateTime = (v: string | null) => (v ? new Date(v).toLocaleString('pt-BR') : '—')
const daysUntil = (v: string) => Math.floor((new Date(v).getTime() - Date.now()) / 86_400_000)

const KEY_LABEL = { ec256: 'ECDSA P-256', rsa2048: 'RSA 2048' }

function StatusBadge({ c }: { c: AcmeCert }) {
  if (c.status === 'issuing')
    return (
      <Badge color="indigo" variant="light" leftSection={<Loader size={10} color="indigo" />}>
        emitindo
      </Badge>
    )
  if (c.status === 'error')
    return (
      <Badge color="red" variant="light" leftSection={<IconX size={12} />}>
        falhou
      </Badge>
    )
  if (c.status === 'pending')
    return (
      <Badge color="gray" variant="light">
        aguardando
      </Badge>
    )
  const days = c.not_after ? daysUntil(c.not_after) : 0
  if (days < 0)
    return (
      <Badge color="red" variant="light">
        vencido
      </Badge>
    )
  return (
    <Badge color={days < 30 ? 'orange' : 'teal'} variant="light" leftSection={<IconCheck size={12} />}>
      válido
    </Badge>
  )
}

function LogTimeline({ log }: { log: LogEntry[] }) {
  return (
    <Timeline bulletSize={22} lineWidth={2} active={log.length - 1}>
      {log.map((e, i) => (
        <Timeline.Item
          key={i}
          color={e.level === 'error' ? 'red' : e.level === 'warn' ? 'orange' : 'indigo'}
          bullet={
            e.level === 'error' ? (
              <IconX size={12} />
            ) : e.level === 'warn' ? (
              <IconAlertTriangle size={12} />
            ) : (
              <IconCheck size={12} />
            )
          }
        >
          <Text size="sm">{e.message}</Text>
          <Text size="xs" c="dimmed">
            {new Date(e.ts).toLocaleTimeString('pt-BR')}
          </Text>
        </Timeline.Item>
      ))}
    </Timeline>
  )
}

function Info({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <Text size="xs" c="dimmed" tt="uppercase" fw={700}>
        {label}
      </Text>
      <Text size="sm" component="div" mt={2}>
        {children}
      </Text>
    </div>
  )
}

function CertCard({
  c,
  admin,
  onIssue,
  onDelete,
  onLog,
}: {
  c: AcmeCert
  admin: boolean
  onIssue: () => void
  onDelete: () => void
  onLog: () => void
}) {
  const days = c.not_after ? daysUntil(c.not_after) : null
  const total =
    c.not_before && c.not_after ? (new Date(c.not_after).getTime() - new Date(c.not_before).getTime()) / 86_400_000 : 90
  const color = days === null ? 'gray' : days < 0 ? 'red' : days < 30 ? 'orange' : 'teal'
  const [first, ...rest] = c.domains

  return (
    <Card>
      <Group justify="space-between" align="flex-start" wrap="nowrap">
        <Group gap="sm" wrap="nowrap" style={{ minWidth: 0 }}>
          <ThemeIcon size={40} radius="md" variant="light" color={color === 'gray' ? 'indigo' : color}>
            <IconCertificate2 size={22} stroke={1.6} />
          </ThemeIcon>
          <div style={{ minWidth: 0 }}>
            <Text fw={700} truncate>
              {first}
            </Text>
            <Group gap={4} mt={2}>
              {rest.slice(0, 4).map((d) => (
                <Badge key={d} size="xs" variant="outline" color="gray">
                  {d}
                </Badge>
              ))}
              {rest.length > 4 && (
                <Badge size="xs" variant="outline" color="gray">
                  +{rest.length - 4}
                </Badge>
              )}
              {c.environment === 'staging' && (
                <Tooltip label="Emitido pelo ambiente de testes: navegadores não confiam neste certificado">
                  <Badge size="xs" color="orange" variant="filled">
                    staging
                  </Badge>
                </Tooltip>
              )}
            </Group>
          </div>
        </Group>
        <Group gap={6} wrap="nowrap">
          <StatusBadge c={c} />
          {admin && (
            <Menu position="bottom-end" shadow="md" width={200}>
              <Menu.Target>
                <ActionIcon variant="subtle" color="gray" aria-label="Ações">
                  <IconDots size={18} />
                </ActionIcon>
              </Menu.Target>
              <Menu.Dropdown>
                <Menu.Item leftSection={<IconListDetails size={16} />} onClick={onLog}>
                  Ver registro da emissão
                </Menu.Item>
                <Menu.Divider />
                <Menu.Item
                  color="red"
                  leftSection={<IconTrash size={16} />}
                  onClick={onDelete}
                  disabled={c.status === 'issuing'}
                >
                  Remover
                </Menu.Item>
              </Menu.Dropdown>
            </Menu>
          )}
        </Group>
      </Group>

      {days !== null && (
        <div style={{ marginTop: 16 }}>
          <Group justify="space-between" mb={4}>
            <Text size="xs" c="dimmed">
              Válido até {fmtDate(c.not_after)}
            </Text>
            <Text size="xs" fw={600} c={color}>
              {days < 0 ? 'vencido' : `${days} dias restantes`}
            </Text>
          </Group>
          <Progress value={Math.max(2, Math.min(100, (days / total) * 100))} color={color} size="sm" radius="xl" />
        </div>
      )}

      {c.status === 'issuing' && c.log.length > 0 && (
        <Card withBorder={false} bg="var(--mantine-color-default-hover)" mt="md" padding="sm">
          <LogTimeline log={c.log.slice(-4)} />
        </Card>
      )}

      {c.status === 'error' && c.last_error && (
        <Alert
          color="red"
          variant="light"
          mt="md"
          icon={<IconAlertCircle size={16} />}
          title={`Falha na emissão${c.failures > 1 ? ` (${c.failures}ª tentativa)` : ''}`}
        >
          <Text size="sm" style={{ wordBreak: 'break-word' }}>
            {c.last_error}
          </Text>
          {c.next_attempt_at && (
            <Text size="xs" c="dimmed" mt={4}>
              Nova tentativa automática em {fmtDateTime(c.next_attempt_at)}
            </Text>
          )}
        </Alert>
      )}

      <SimpleGrid cols={{ base: 2, sm: 3 }} mt="md" spacing="sm" verticalSpacing="sm">
        <Info label="SSL no APISIX">
          <Anchor component={Link} to="/r/ssls" size="sm" ff="monospace">
            {c.ssl_id}
          </Anchor>
        </Info>
        <Info label="Renovação">{c.renew_at ? fmtDate(c.renew_at) : 'após a emissão'}</Info>
        <Info label="Chave">{KEY_LABEL[c.key_type]}</Info>
        <Info label="Emissor">{c.issuer || '—'}</Info>
        <Info label="Última emissão">{fmtDateTime(c.issued_at)}</Info>
        <Info label="SHA-256">
          {c.fingerprint ? (
            <Tooltip label={c.fingerprint} maw={360} multiline>
              <Code>{c.fingerprint.slice(0, 16)}…</Code>
            </Tooltip>
          ) : (
            '—'
          )}
        </Info>
      </SimpleGrid>

      {admin && (
        <Group justify="flex-end" mt="md" gap="xs">
          <Button size="xs" variant="default" leftSection={<IconListDetails size={14} />} onClick={onLog}>
            Registro
          </Button>
          <Button
            size="xs"
            variant="light"
            leftSection={<IconRefresh size={14} />}
            onClick={onIssue}
            loading={c.status === 'issuing'}
          >
            {c.status === 'valid' ? 'Renovar agora' : 'Emitir agora'}
          </Button>
        </Group>
      )}
    </Card>
  )
}

function ProviderCard({ info }: { info: AcmeInfo }) {
  const q = useQuery({
    queryKey: ['acme', 'provider'],
    queryFn: () => api<Provider>('GET', '/api/acme/provider'),
    staleTime: 60_000,
  })
  return (
    <Card>
      <Group justify="space-between" align="flex-start">
        <Group gap="sm" wrap="nowrap">
          <ThemeIcon size={40} radius="md" variant="light" color="orange">
            <IconCloud size={22} stroke={1.6} />
          </ThemeIcon>
          <div>
            <Text fw={700}>Let's Encrypt · DNS-01 via Cloudflare</Text>
            <Text size="sm" c="dimmed">
              Renova {info.renew_before_days} dias antes do vencimento
              {info.email ? ` · conta ${info.email}` : ' · sem e-mail de contato'}
            </Text>
          </div>
        </Group>
        {q.isPending ? (
          <Skeleton h={22} w={110} />
        ) : q.data?.token_ok ? (
          <Badge color="teal" variant="light" leftSection={<IconCircleCheck size={12} />}>
            token válido
          </Badge>
        ) : (
          <Badge color="red" variant="light">
            token inválido
          </Badge>
        )}
      </Group>
      {q.data?.error && (
        <Alert color="red" variant="light" mt="md" icon={<IconAlertCircle size={16} />}>
          {q.data.error}
        </Alert>
      )}
      {q.data?.token_ok && (
        <Group gap={6} mt="md">
          <Text size="xs" c="dimmed" fw={700} tt="uppercase">
            Zonas acessíveis
          </Text>
          {q.data.zones.length === 0 ? (
            <Text size="sm" c="dimmed">
              nenhuma: dê ao token permissão Zone:DNS:Edit
            </Text>
          ) : (
            q.data.zones.map((z) => (
              <Badge key={z.name} variant="outline" color={z.status === 'active' ? 'indigo' : 'gray'}>
                {z.name}
              </Badge>
            ))
          )}
        </Group>
      )}
    </Card>
  )
}

function SetupHelp() {
  return (
    <Card>
      <EmptyState
        icon={IconCertificate2}
        title="Emissão automática desativada"
        description="A dashboard emite e renova certificados Let's Encrypt pelo desafio DNS na Cloudflare e publica no APISIX, sem reload. Para ativar, configure o serviço:"
      />
      <Code block mx="auto" maw={640}>
        {`DASH_ACME_CLOUDFLARE_API_TOKEN_FILE=/run/secrets/cloudflare_token
DASH_ACME_EMAIL=infra@exemplo.com.br   # opcional`}
      </Code>
      <Text size="xs" c="dimmed" ta="center" mt="sm" mb="md">
        Crie o token na Cloudflare com permissão <b>Zone · DNS · Edit</b> apenas nas zonas usadas.
      </Text>
    </Card>
  )
}

function CreateModal({ opened, onClose, existing }: { opened: boolean; onClose: () => void; existing: AcmeCert[] }) {
  const qc = useQueryClient()
  const [domains, setDomains] = useState<string[]>([])
  const [env, setEnv] = useState('production')
  const [keyType, setKeyType] = useState('ec256')
  const [sslId, setSslId] = useState('')
  const [replace, setReplace] = useState(false)
  const [error, setError] = useState('')
  const ssls = useQuery({
    queryKey: ['apisix', 'ssls'],
    queryFn: () => api<AdminList>('GET', '/api/apisix/ssls'),
    enabled: opened,
  })

  const defaultId = domains[0]
    ? `acme-${domains[0].toLowerCase().replace('*', 'wildcard').replaceAll('.', '-')}`.slice(0, 64)
    : ''
  const effectiveId = sslId.trim() || defaultId
  const managedIds = new Set(existing.map((c) => c.ssl_id))
  const alreadyManaged = managedIds.has(effectiveId)
  const sameId = !alreadyManaged && (ssls.data?.list ?? []).some((i) => itemId('ssls', i) === effectiveId)
  const overlapping = useMemo(() => {
    const wanted = new Set(domains.map((d) => d.toLowerCase()))
    return (ssls.data?.list ?? [])
      .map((i) => ({
        id: itemId('ssls', i),
        snis: ([] as unknown[]).concat(i.value.snis ?? i.value.sni ?? []).map(String),
      }))
      .filter((s) => s.id !== effectiveId && !managedIds.has(s.id) && s.snis.some((n) => wanted.has(n.toLowerCase())))
  }, [ssls.data, domains, effectiveId, managedIds])

  const create = useMutation({
    mutationFn: () =>
      api('POST', '/api/acme/certificates', {
        domains,
        environment: env,
        key_type: keyType,
        ssl_id: sslId.trim(),
        replace,
      }),
    onSuccess: () => {
      notifications.show({
        color: 'indigo',
        icon: <IconClockHour4 size={18} />,
        title: 'Emissão iniciada',
        message: 'Acompanhe o progresso no card do certificado.',
      })
      void qc.invalidateQueries({ queryKey: ['acme'] })
      close()
    },
    onError: (e) => setError(e.message),
  })

  const close = () => {
    setDomains([])
    setEnv('production')
    setKeyType('ec256')
    setSslId('')
    setReplace(false)
    setError('')
    onClose()
  }

  return (
    <Modal opened={opened} onClose={close} size="lg" title={<Text fw={700}>Novo certificado automático</Text>}>
      <Stack>
        <TagsInput
          label="Domínios"
          withAsterisk
          description="Digite e tecle Enter. Wildcard é suportado: *.exemplo.com.br"
          placeholder="exemplo.com.br"
          value={domains}
          onChange={setDomains}
          splitChars={[',', ' ']}
          clearable
        />
        <div>
          <Text size="sm" fw={500} mb={4}>
            Ambiente
          </Text>
          <SegmentedControl
            fullWidth
            value={env}
            onChange={setEnv}
            data={[
              { value: 'production', label: 'Produção' },
              { value: 'staging', label: 'Staging (teste)' },
            ]}
          />
          <Text size="xs" c="dimmed" mt={4}>
            {env === 'staging'
              ? "Ambiente de testes do Let's Encrypt: valida todo o fluxo sem consumir os limites, mas o certificado não é confiável."
              : 'Certificado confiável pelos navegadores. Use staging antes se estiver testando.'}
          </Text>
        </div>
        <Group grow align="flex-start">
          <Select
            label="Tipo de chave"
            value={keyType}
            onChange={(v) => setKeyType(v ?? 'ec256')}
            allowDeselect={false}
            data={[
              { value: 'ec256', label: 'ECDSA P-256 (recomendado)' },
              { value: 'rsa2048', label: 'RSA 2048 (clientes antigos)' },
            ]}
          />
          <TextInput
            label="ID do SSL no APISIX"
            placeholder={defaultId || 'gerado a partir do domínio'}
            value={sslId}
            onChange={(e) => setSslId(e.currentTarget.value)}
          />
        </Group>

        {alreadyManaged && (
          <Alert
            color="red"
            variant="light"
            icon={<IconAlertCircle size={16} />}
            title={`O SSL ${effectiveId} já é emitido automaticamente`}
          >
            <Text size="sm">
              Escolha outro ID, ou edite os domínios removendo e cadastrando o certificado existente.
            </Text>
          </Alert>
        )}
        {sameId && (
          <Alert
            color="orange"
            variant="light"
            icon={<IconAlertTriangle size={16} />}
            title={`Já existe o SSL ${effectiveId}`}
          >
            <Text size="sm">
              Ele continua servindo o tráfego até o novo certificado ser emitido e só então é substituído. Use isso para
              migrar um certificado gerenciado manualmente.
            </Text>
            <Checkbox
              mt="sm"
              label="Substituir o SSL existente"
              checked={replace}
              onChange={(e) => setReplace(e.currentTarget.checked)}
            />
          </Alert>
        )}
        {overlapping.length > 0 && (
          <Alert
            color="yellow"
            variant="light"
            icon={<IconInfoCircle size={16} />}
            title="SNIs já atendidos por outro SSL"
          >
            <Text size="sm">
              {overlapping.map((o) => o.id).join(', ')} também responde por {domains.join(', ')}. Depois da emissão,
              exclua o SSL antigo para o APISIX usar apenas o novo.
            </Text>
          </Alert>
        )}
        {error && (
          <Alert color="red" variant="light" icon={<IconAlertCircle size={16} />}>
            {error}
          </Alert>
        )}
        <Text size="xs" c="dimmed">
          Ao emitir, você aceita os termos de uso do Let's Encrypt. A chave privada é gerada aqui, enviada ao APISIX e
          nunca armazenada pela dashboard.
        </Text>
        <Group justify="flex-end">
          <Button variant="default" onClick={close}>
            Cancelar
          </Button>
          <Button
            onClick={() => create.mutate()}
            loading={create.isPending}
            disabled={domains.length === 0 || alreadyManaged || (sameId && !replace)}
          >
            Emitir certificado
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}

export function Certificates() {
  const { me } = useAuth()
  const admin = isAdmin(me.role)
  const qc = useQueryClient()
  const [creating, setCreating] = useState(false)
  const [logOf, setLogOf] = useState<number | null>(null)
  const [deleting, setDeleting] = useState<AcmeCert | null>(null)
  const [deleteSSL, setDeleteSSL] = useState(false)

  const q = useQuery({
    queryKey: ['acme'],
    queryFn: () => api<AcmeInfo>('GET', '/api/acme'),
    refetchInterval: (query) => {
      const certs = query.state.data?.certificates ?? []
      return certs.some((c) => c.status === 'issuing') ? 1500 : 30_000
    },
  })
  const certs = q.data?.certificates ?? []
  const logCert = certs.find((c) => c.id === logOf)

  const issue = useMutation({
    mutationFn: (id: number) => api('POST', `/api/acme/certificates/${id}/issue`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['acme'] }),
    onError: (e) => notifications.show({ color: 'red', title: 'Não foi possível iniciar', message: e.message }),
  })
  const remove = useMutation({
    mutationFn: ({ id, ssl }: { id: number; ssl: boolean }) =>
      api('DELETE', `/api/acme/certificates/${id}${ssl ? '?delete_ssl=true' : ''}`),
    onSuccess: () => {
      notifications.show({
        color: 'teal',
        icon: <IconCheck size={18} />,
        title: 'Removido',
        message: 'O certificado não será mais renovado.',
      })
      void qc.invalidateQueries({ queryKey: ['acme'] })
      void qc.invalidateQueries({ queryKey: ['apisix', 'ssls'] })
      setDeleting(null)
    },
    onError: (e) => notifications.show({ color: 'red', title: 'Falha ao remover', message: e.message }),
  })

  // Refresh the SSL list once an issuance finishes.
  const issuing = certs.filter((c) => c.status === 'issuing').length
  const prevIssuing = useRef(0)
  useEffect(() => {
    if (issuing < prevIssuing.current) void qc.invalidateQueries({ queryKey: ['apisix', 'ssls'] })
    prevIssuing.current = issuing
  }, [issuing, qc])

  return (
    <>
      <PageHeader
        icon={IconCertificate2}
        title="Certificados automáticos"
        description="Emissão e renovação Let's Encrypt pelo desafio DNS, publicadas no APISIX sem reload"
        badge={
          q.data?.enabled && (
            <Badge variant="light" size="lg" radius="xl">
              {certs.length}
            </Badge>
          )
        }
        actions={
          admin &&
          q.data?.enabled && (
            <Button leftSection={<IconPlus size={16} />} onClick={() => setCreating(true)}>
              Novo certificado
            </Button>
          )
        }
      />

      {q.isPending ? (
        <Skeleton h={160} />
      ) : q.error ? (
        <Alert color="red" icon={<IconAlertCircle size={16} />} title="Erro ao carregar">
          {q.error.message}
        </Alert>
      ) : !q.data?.enabled ? (
        <SetupHelp />
      ) : (
        <Stack>
          {admin && <ProviderCard info={q.data} />}
          {certs.length === 0 ? (
            <Card>
              <EmptyState
                icon={IconCertificate2}
                title="Nenhum certificado automático"
                description="Cadastre os domínios: a dashboard cria o TXT na Cloudflare, valida, emite e publica no APISIX. A renovação é automática."
                action={
                  admin && (
                    <Button variant="light" leftSection={<IconPlus size={16} />} onClick={() => setCreating(true)}>
                      Emitir o primeiro
                    </Button>
                  )
                }
              />
            </Card>
          ) : (
            <SimpleGrid cols={{ base: 1, lg: 2 }}>
              {certs.map((c) => (
                <CertCard
                  key={c.id}
                  c={c}
                  admin={admin}
                  onIssue={() => issue.mutate(c.id)}
                  onLog={() => setLogOf(c.id)}
                  onDelete={() => {
                    setDeleteSSL(false)
                    setDeleting(c)
                  }}
                />
              ))}
            </SimpleGrid>
          )}
        </Stack>
      )}

      <CreateModal opened={creating} onClose={() => setCreating(false)} existing={certs} />

      <Modal
        opened={!!logCert}
        onClose={() => setLogOf(null)}
        title={<Text fw={700}>Registro da última emissão</Text>}
        size="lg"
      >
        {logCert && (
          <Stack>
            <Text size="sm" c="dimmed">
              {logCert.domains.join(', ')}
            </Text>
            {logCert.log.length ? <LogTimeline log={logCert.log} /> : <Text size="sm">Ainda não houve emissão.</Text>}
          </Stack>
        )}
      </Modal>

      <Modal
        opened={!!deleting}
        onClose={() => setDeleting(null)}
        title={<Text fw={700}>Remover certificado automático</Text>}
      >
        {deleting && (
          <Stack>
            <Text size="sm">
              <b>{deleting.domains.join(', ')}</b> deixará de ser renovado.
            </Text>
            <Checkbox
              label={`Excluir também o SSL ${deleting.ssl_id} do APISIX`}
              description="Sem isso, o certificado atual continua sendo servido até vencer e passa a ser editável como um SSL comum."
              checked={deleteSSL}
              onChange={(e) => setDeleteSSL(e.currentTarget.checked)}
              color="red"
            />
            <Group justify="flex-end">
              <Button variant="default" onClick={() => setDeleting(null)}>
                Cancelar
              </Button>
              <Button
                color="red"
                leftSection={<IconTrash size={16} />}
                loading={remove.isPending}
                onClick={() => remove.mutate({ id: deleting.id, ssl: deleteSSL })}
              >
                Remover
              </Button>
            </Group>
          </Stack>
        )}
      </Modal>
    </>
  )
}
