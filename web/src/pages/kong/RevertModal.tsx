// Reverts migrated objects: deletes what the migration created and restores
// what it replaced, after a preview and a typed confirmation.
import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Alert,
  Badge,
  Button,
  Group,
  Loader,
  Modal,
  ScrollArea,
  Stack,
  Switch,
  Table,
  Text,
  TextInput,
  Tooltip,
} from '@mantine/core'
import { useMediaQuery } from '@mantine/hooks'
import { IconAlertCircle, IconAlertTriangle, IconArrowBackUp } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'
import { api } from '../../api'
import { serverText } from '../../i18n/format'
import { KIND_LABEL, type RevertAction, type RevertItem, type RevertPlan, type RevertResult } from './types'

const ACTION_COLOR: Record<RevertAction, string> = { delete: 'red', restore: 'blue', manual: 'orange', missing: 'gray' }
const ACTION_LABEL = {
  delete: 'kong.revert.action_delete',
  restore: 'kong.revert.action_restore',
  manual: 'kong.revert.action_manual',
  missing: 'kong.revert.action_missing',
} as const

const acts = (it: RevertItem, includeChanged: boolean) =>
  (it.action === 'delete' || it.action === 'restore') && (includeChanged || !it.changed)

export function RevertModal({
  recordIds,
  opened,
  onClose,
}: {
  recordIds: number[]
  opened: boolean
  onClose: () => void
}) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const mobile = useMediaQuery('(max-width: 48em)')
  const [includeChanged, setIncludeChanged] = useState(false)
  const [confirm, setConfirm] = useState('')

  const plan = useQuery({
    queryKey: ['kong', 'revert-plan', recordIds],
    queryFn: () => api<RevertPlan>('POST', '/api/migration/kong/revert/plan', { record_ids: recordIds }),
    enabled: opened && recordIds.length > 0,
    staleTime: 0,
    gcTime: 0,
  })
  const items = useMemo(() => plan.data?.items ?? [], [plan.data])
  const count = (a: RevertAction) => items.filter((i) => i.action === a).length
  const changed = items.filter((i) => i.changed && (i.action === 'delete' || i.action === 'restore')).length
  const affected = items.filter((i) => acts(i, includeChanged)).length
  const missing = count('missing')

  const revert = useMutation({
    mutationFn: () =>
      api<{ results: RevertResult[] }>('POST', '/api/migration/kong/revert', {
        record_ids: recordIds,
        hash: plan.data?.hash,
        include_changed: includeChanged,
        confirm: Number(confirm),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['kong'] }),
  })
  const results = revert.data?.results
  const failures = results?.filter((r) => r.outcome === 'failure') ?? []

  const close = () => {
    setConfirm('')
    setIncludeChanged(false)
    revert.reset()
    onClose()
  }
  const canRevert = !!plan.data && (affected > 0 || missing > 0) && confirm.trim() === String(affected)

  return (
    <Modal
      opened={opened}
      onClose={close}
      size="xl"
      fullScreen={mobile}
      title={<Text fw={700}>{t('kong.revert.title', { count: recordIds.length })}</Text>}
    >
      {plan.isPending ? (
        <Group justify="center" p="xl">
          <Loader />
        </Group>
      ) : plan.error ? (
        <Alert color="red" variant="light" icon={<IconAlertCircle size={16} />}>
          {plan.error.message}
        </Alert>
      ) : results ? (
        <Stack>
          <Alert color={failures.length ? 'orange' : 'teal'} variant="light">
            {t('kong.revert.done', {
              ok: results.filter((r) => r.outcome === 'success').length,
              failed: failures.length,
              skipped: results.filter((r) => r.outcome === 'skipped').length,
            })}
          </Alert>
          {failures.map((r) => (
            <Text key={r.path} size="sm" c="red">
              <Text span ff="monospace" size="sm">
                {r.path}
              </Text>
              : {serverText(r.error)}
            </Text>
          ))}
          <Group justify="flex-end">
            <Button onClick={close}>{t('common.close')}</Button>
          </Group>
        </Stack>
      ) : (
        <Stack>
          <Alert
            color="red"
            variant="light"
            icon={<IconAlertTriangle size={16} />}
            title={t('kong.revert.warning_title')}
          >
            {t('kong.revert.warning')}
          </Alert>
          <Group gap="xs">
            {(['delete', 'restore', 'manual', 'missing'] as const).map((a) => (
              <Badge key={a} color={ACTION_COLOR[a]} variant="light">
                {t(ACTION_LABEL[a])}: {count(a)}
              </Badge>
            ))}
          </Group>
          {changed > 0 && (
            <Switch
              checked={includeChanged}
              onChange={(e) => setIncludeChanged(e.currentTarget.checked)}
              label={t('kong.revert.include_changed', { count: changed })}
              description={t('kong.revert.include_changed_desc')}
            />
          )}
          <ScrollArea.Autosize mah={360}>
            <Table verticalSpacing={4} fz="sm">
              <Table.Thead>
                <Table.Tr>
                  <Table.Th>{t('kong.col_kind')}</Table.Th>
                  <Table.Th>{t('kong.col_in_apisix')}</Table.Th>
                  <Table.Th>{t('kong.revert.col_action')}</Table.Th>
                </Table.Tr>
              </Table.Thead>
              <Table.Tbody>
                {items.map((it) => (
                  <Table.Tr key={it.record_id} opacity={acts(it, includeChanged) || it.action === 'missing' ? 1 : 0.55}>
                    <Table.Td>
                      <Badge size="xs" variant="outline" color="gray">
                        {t(KIND_LABEL[it.kind])}
                      </Badge>
                    </Table.Td>
                    <Table.Td>
                      <Tooltip label={it.kong_name}>
                        <Text ff="monospace" size="sm">
                          {it.path}
                        </Text>
                      </Tooltip>
                    </Table.Td>
                    <Table.Td>
                      <Group gap={4}>
                        <Badge size="sm" color={ACTION_COLOR[it.action]} variant="light">
                          {t(ACTION_LABEL[it.action])}
                        </Badge>
                        {it.changed && (it.action === 'delete' || it.action === 'restore') && (
                          <Badge size="sm" color="yellow" variant="outline">
                            {t('kong.revert.changed')}
                          </Badge>
                        )}
                      </Group>
                      {it.reason && (
                        <Text size="xs" c="dimmed">
                          {serverText(it.reason)}
                        </Text>
                      )}
                    </Table.Td>
                  </Table.Tr>
                ))}
              </Table.Tbody>
            </Table>
          </ScrollArea.Autosize>
          <TextInput
            label={t('kong.revert.confirm_label', { count: affected })}
            placeholder={String(affected)}
            value={confirm}
            onChange={(e) => setConfirm(e.currentTarget.value)}
            inputMode="numeric"
          />
          {revert.error && (
            <Alert color="red" variant="light" icon={<IconAlertCircle size={16} />}>
              {revert.error.message}
            </Alert>
          )}
          <Group justify="flex-end">
            <Button variant="default" onClick={close}>
              {t('common.cancel')}
            </Button>
            <Button
              color="red"
              leftSection={<IconArrowBackUp size={16} />}
              disabled={!canRevert}
              loading={revert.isPending}
              onClick={() => revert.mutate()}
            >
              {t('kong.revert.submit', { count: affected })}
            </Button>
          </Group>
        </Stack>
      )}
    </Modal>
  )
}
