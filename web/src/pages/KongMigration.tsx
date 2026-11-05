import { useMemo, useState } from 'react'
import { Link } from 'react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Alert,
  Anchor,
  Badge,
  Button,
  Card,
  Code,
  Skeleton,
  Stack,
  Table,
  Tabs,
  Text,
  TextInput,
  Tooltip,
} from '@mantine/core'
import {
  IconAdjustments,
  IconAlertCircle,
  IconHistory,
  IconListCheck,
  IconRefresh,
  IconSearch,
  IconTransfer,
} from '@tabler/icons-react'
import { api } from '../api'
import { EmptyState } from '../components/EmptyState'
import { PageHeader } from '../components/PageHeader'
import { Inventory } from './kong/Inventory'
import { PreviewModal } from './kong/PreviewModal'
import { SettingsForm } from './kong/SettingsForm'
import { KIND_LABEL, type KongInfo, type MigrationRecord, type Plan, type Selection } from './kong/types'
import { Trans, useTranslation } from 'react-i18next'
import { formatDateTime, serverText } from '../i18n/format'

function SetupHelp() {
  const { t } = useTranslation()
  return (
    <Card>
      <EmptyState icon={IconTransfer} title={t('kong.setup_title')} description={t('kong.setup_desc')} />
      <Code block mx="auto" maw={640}>
        {`DASH_KONG_ADMIN_URL=http://kong-admin:8001
DASH_KONG_ADMIN_TOKEN_FILE=/run/secrets/kong_admin_token   # ${t('kong.setup_token_comment')}`}
      </Code>
      <Text size="xs" c="dimmed" ta="center" mt="sm" mb="md">
        {t('kong.setup_hint')}
      </Text>
    </Card>
  )
}

function History() {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')
  const q = useQuery({
    queryKey: ['kong', 'history'],
    queryFn: () => api<MigrationRecord[]>('GET', '/api/migration/kong/history'),
  })
  const rows = useMemo(() => {
    const s = query.trim().toLowerCase()
    return (q.data ?? []).filter(
      (r) => !s || r.kong_name.toLowerCase().includes(s) || r.id.toLowerCase().includes(s) || r.migrated_by.includes(s),
    )
  }, [q.data, query])
  if (q.isPending) return <Skeleton h={200} />
  if (!q.data?.length)
    return (
      <Card>
        <EmptyState icon={IconHistory} title={t('kong.history_empty')} description={t('kong.history_empty_desc')} />
      </Card>
    )
  return (
    <Card>
      <TextInput
        placeholder={t('common.search')}
        leftSection={<IconSearch size={16} />}
        value={query}
        onChange={(e) => setQuery(e.currentTarget.value)}
        mb="md"
        maw={360}
      />
      <Table.ScrollContainer minWidth={640}>
        <Table verticalSpacing={6} fz="sm" highlightOnHover>
          <Table.Thead>
            <Table.Tr>
              <Table.Th>{t('kong.col_when')}</Table.Th>
              <Table.Th>{t('kong.col_kind')}</Table.Th>
              <Table.Th>{t('kong.col_in_kong')}</Table.Th>
              <Table.Th>{t('kong.col_in_apisix')}</Table.Th>
              <Table.Th>{t('kong.col_by')}</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {rows.map((r) => (
              <Table.Tr key={r.kind + r.kong_id}>
                <Table.Td>{formatDateTime(r.migrated_at)}</Table.Td>
                <Table.Td>
                  <Badge size="xs" variant="outline" color="gray">
                    {t(KIND_LABEL[r.kind])}
                  </Badge>
                </Table.Td>
                <Table.Td>
                  <Tooltip label={r.kong_id}>
                    <Text size="sm">{r.kong_name}</Text>
                  </Tooltip>
                </Table.Td>
                <Table.Td>
                  <Anchor component={Link} to={`/r/${r.resource}`} ff="monospace" size="sm">
                    {r.resource}/{r.id}
                  </Anchor>
                </Table.Td>
                <Table.Td>{r.migrated_by}</Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      </Table.ScrollContainer>
    </Card>
  )
}

export function KongMigration() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const [tab, setTab] = useState<string | null>('inventory')
  const [preview, setPreview] = useState<Selection | null>(null)
  const info = useQuery({ queryKey: ['kong', 'info'], queryFn: () => api<KongInfo>('GET', '/api/migration/kong') })
  const inventory = useQuery({
    queryKey: ['kong', 'inventory'],
    queryFn: () => api<Plan>('GET', '/api/migration/kong/inventory'),
    enabled: !!info.data?.enabled,
    staleTime: 60_000,
  })

  // Kong plugins in use, offered when writing custom rules.
  const kongPlugins = useMemo(
    () => [...new Set((inventory.data?.items ?? []).flatMap((i) => i.plugins?.map((p) => p.kong) ?? []))].toSorted(),
    [inventory.data],
  )
  const untranslated = useMemo(
    () =>
      [
        ...new Set(
          (inventory.data?.items ?? []).flatMap(
            (i) => i.plugins?.filter((p) => p.status === 'error' && !p.custom).map((p) => p.kong) ?? [],
          ),
        ),
      ].toSorted(),
    [inventory.data],
  )

  const k = info.data
  return (
    <>
      <PageHeader
        icon={IconTransfer}
        title={t('kong.title')}
        description={t('kong.description')}
        badge={
          k?.info && (
            <Tooltip label={k.url}>
              <Badge variant="light" color="teal">
                Kong {k.info.version}
              </Badge>
            </Tooltip>
          )
        }
        actions={
          k?.enabled && (
            <Button
              variant="default"
              leftSection={<IconRefresh size={16} />}
              loading={inventory.isFetching}
              onClick={() => qc.invalidateQueries({ queryKey: ['kong'] })}
            >
              {t('kong.reload')}
            </Button>
          )
        }
      />
      {info.isPending ? (
        <Skeleton h={240} />
      ) : !k?.enabled ? (
        <SetupHelp />
      ) : (
        <Stack>
          {k.error && (
            <Alert color="red" variant="light" icon={<IconAlertCircle size={16} />}>
              {serverText({ code: k.code, params: k.params, message: k.error })}
            </Alert>
          )}
          {untranslated.length > 0 && (
            <Alert
              color="orange"
              variant="light"
              icon={<IconAlertCircle size={16} />}
              title={t('kong.untranslated_title')}
            >
              <Text size="sm">
                {untranslated.map((p, i) => (
                  <span key={p}>
                    {i > 0 && ', '}
                    <Code>{p}</Code>
                  </span>
                ))}
                .{' '}
                <Trans
                  i18nKey="kong.untranslated_hint"
                  components={{ link: <Anchor component="button" size="sm" onClick={() => setTab('settings')} /> }}
                />
              </Text>
            </Alert>
          )}
          <Tabs value={tab} onChange={setTab} keepMounted={false}>
            <Tabs.List mb="md">
              <Tabs.Tab value="inventory" leftSection={<IconListCheck size={16} />}>
                {t('kong.tab_inventory')}
              </Tabs.Tab>
              <Tabs.Tab value="settings" leftSection={<IconAdjustments size={16} />}>
                {t('kong.tab_settings')}
              </Tabs.Tab>
              <Tabs.Tab value="history" leftSection={<IconHistory size={16} />}>
                {t('kong.tab_history')}
              </Tabs.Tab>
            </Tabs.List>
            <Tabs.Panel value="inventory">
              {inventory.isPending ? (
                <Stack>
                  <Skeleton h={90} />
                  <Skeleton h={300} />
                </Stack>
              ) : inventory.error ? (
                <Alert
                  color="red"
                  variant="light"
                  icon={<IconAlertCircle size={16} />}
                  title={t('kong.inventory_error')}
                >
                  {inventory.error.message}
                </Alert>
              ) : (
                <Inventory plan={inventory.data} onPreview={setPreview} />
              )}
            </Tabs.Panel>
            <Tabs.Panel value="settings">
              {k.settings && <SettingsForm settings={k.settings} kongPlugins={kongPlugins} />}
            </Tabs.Panel>
            <Tabs.Panel value="history">
              <History />
            </Tabs.Panel>
          </Tabs>
        </Stack>
      )}
      <PreviewModal selection={preview} opened={!!preview} onClose={() => setPreview(null)} />
    </>
  )
}
