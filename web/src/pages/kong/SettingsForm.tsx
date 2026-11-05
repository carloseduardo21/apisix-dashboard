import { useMemo, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  ActionIcon,
  Alert,
  Autocomplete,
  Badge,
  Button,
  Card,
  Code,
  Group,
  JsonInput,
  SegmentedControl,
  SimpleGrid,
  Stack,
  Switch,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { notifications } from '@mantine/notifications'
import { IconAlertCircle, IconArrowRight, IconCheck, IconDeviceFloppy, IconPlus, IconTrash } from '@tabler/icons-react'
import { api } from '../../api'
import { KeyValueEditor } from '../../schema/SchemaForm'
import { useCatalog } from '../../schema/types'
import type { PluginRule, Settings } from './types'
import { errorMessage, stringRecord } from '../../values'
import { Trans, useTranslation } from 'react-i18next'

function Choice<T extends string>({
  label,
  description,
  value,
  onChange,
  data,
}: {
  label: string
  description: string
  value: T
  onChange: (v: T) => void
  data: { value: T; label: string }[]
}) {
  return (
    <div>
      <Text size="sm" fw={600}>
        {label}
      </Text>
      <Text size="xs" c="dimmed" mb={6}>
        {description}
      </Text>
      <SegmentedControl fullWidth value={value} onChange={(v) => onChange(v)} data={data} />
    </div>
  )
}

const TEMPLATE_EXAMPLE = `{
  "level": "\${config.level}",
  "limit": "\${config.limit|10}",
  "route": "\${apisix.route_id}"
}`

// Template syntax samples rendered inside the translated help text.
const code = (s: string) => <Code>{s}</Code>
const TEMPLATE_HELP_PARTS = {
  field: code('${config.field}'),
  fielddefault: code('${config.field|default}'),
  config: code('${config}'),
  plugin: code('plugin'),
  service: code('service'),
  route: code('route'),
  consumer: code('consumer'),
  rid: code('apisix.route_id'),
  sid: code('apisix.service_id'),
  uid: code('apisix.upstream_id'),
  cid: code('apisix.consumer'),
}

function RuleEditor({
  rule,
  onChange,
  onRemove,
  kongPlugins,
  apisixPlugins,
}: {
  rule: PluginRule
  onChange: (r: PluginRule) => void
  onRemove: () => void
  kongPlugins: string[]
  apisixPlugins: string[]
}) {
  const { t } = useTranslation()
  // The template is edited as text and only parsed when valid.
  const [text, setText] = useState(() => (rule.template ? JSON.stringify(rule.template, null, 2) : ''))
  const [error, setError] = useState('')
  const parse = (v: string) => {
    setText(v)
    if (!v.trim()) {
      setError('')
      onChange({ ...rule, template: undefined })
      return
    }
    try {
      const tpl = JSON.parse(v)
      if (!tpl || typeof tpl !== 'object' || Array.isArray(tpl)) throw new Error(t('kong.set.template_not_object'))
      setError('')
      onChange({ ...rule, template: tpl })
    } catch (e) {
      setError(errorMessage(e))
    }
  }
  return (
    <Card withBorder padding="md">
      <Group align="flex-end" wrap="wrap" gap="sm">
        <Autocomplete
          label={t('kong.set.kong_plugin')}
          placeholder="my-plugin"
          data={kongPlugins}
          value={rule.kong_plugin}
          onChange={(v) => onChange({ ...rule, kong_plugin: v.trim() })}
          style={{ flex: 1, minWidth: 180 }}
        />
        <SegmentedControl
          value={rule.action}
          onChange={(v) => onChange({ ...rule, action: v })}
          data={[
            { value: 'map', label: t('kong.set.map') },
            { value: 'ignore', label: t('kong.set.ignore') },
          ]}
        />
        {rule.action === 'map' && (
          <>
            <IconArrowRight size={18} style={{ marginBottom: 8 }} />
            <Autocomplete
              label={t('kong.set.apisix_plugin')}
              placeholder="my-apisix-plugin"
              data={apisixPlugins}
              value={rule.apisix_plugin ?? ''}
              onChange={(v) => onChange({ ...rule, apisix_plugin: v.trim() })}
              style={{ flex: 1, minWidth: 180 }}
            />
          </>
        )}
        <ActionIcon
          variant="subtle"
          color="red"
          size="lg"
          onClick={onRemove}
          aria-label={t('kong.set.remove_rule')}
          mb={2}
        >
          <IconTrash size={18} />
        </ActionIcon>
      </Group>
      {rule.action === 'map' && (
        <JsonInput
          mt="sm"
          label={t('kong.set.template')}
          description={t('kong.set.template_hint')}
          placeholder={TEMPLATE_EXAMPLE}
          value={text}
          onChange={parse}
          error={error}
          autosize
          minRows={4}
          maxRows={16}
          formatOnBlur
          styles={{ input: { fontFamily: 'var(--mantine-font-family-monospace)', fontSize: 13 } }}
        />
      )}
    </Card>
  )
}

/** Migration defaults, applied to every step. */
export function SettingsForm({ settings, kongPlugins }: { settings: Settings; kongPlugins: string[] }) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const catalog = useCatalog()
  const [draft, setDraft] = useState<Settings>(settings)
  const [error, setError] = useState('')
  // Remounts the key/value editor when the saved settings are reloaded.
  const [version, setVersion] = useState(0)
  // Stable React keys for plugin rules: RuleEditor keeps the template text in local
  // state, so index keys would hand one rule's text to another after a removal.
  const [ruleKeys, setRuleKeys] = useState<number[]>(() => settings.plugin_rules.map((_, i) => i))
  const [nextRuleKey, setNextRuleKey] = useState(settings.plugin_rules.length)
  const [prevSettings, setPrevSettings] = useState(settings)
  if (settings !== prevSettings) {
    setPrevSettings(settings)
    setDraft(settings)
    setVersion((v) => v + 1)
    setRuleKeys(settings.plugin_rules.map((_, i) => nextRuleKey + i))
    setNextRuleKey(nextRuleKey + settings.plugin_rules.length)
  }

  const apisixPlugins = useMemo(() => Object.keys(catalog.data?.plugins ?? {}).toSorted(), [catalog.data])
  const dirty = JSON.stringify(draft) !== JSON.stringify(settings)
  const set = <K extends keyof Settings>(k: K, v: Settings[K]) => setDraft((d) => ({ ...d, [k]: v }))

  const save = useMutation({
    mutationFn: () => api<Settings>('PUT', '/api/migration/kong/settings', draft),
    // oxlint-disable-next-line react/no-unstable-nested-components -- mutation callback that builds a notification icon element, not a component
    onSuccess: () => {
      setError('')
      notifications.show({
        color: 'teal',
        icon: <IconCheck size={18} />,
        title: t('kong.set.saved_title'),
        message: t('kong.set.saved'),
      })
      void qc.invalidateQueries({ queryKey: ['kong'] })
    },
    onError: (e) => setError(e.message),
  })

  const exampleId = `${draft.id_prefix}${draft.id_strategy === 'name' ? 'payments-api' : '5d34560d-8c3c-…'}`

  return (
    <Stack>
      <SimpleGrid cols={{ base: 1, md: 2 }}>
        <Card>
          <Title order={4} mb="md">
            {t('kong.set.identification')}
          </Title>
          <Stack>
            <Choice
              label={t('kong.set.id_label')}
              description={t('kong.set.id_hint')}
              value={draft.id_strategy}
              onChange={(v) => set('id_strategy', v)}
              data={[
                { value: 'name', label: t('kong.set.id_name') },
                { value: 'kong_id', label: t('kong.set.id_kong') },
              ]}
            />
            <TextInput
              label={t('kong.set.prefix')}
              description={t('kong.set.prefix_hint')}
              placeholder="kong-"
              maxLength={20}
              value={draft.id_prefix}
              onChange={(e) => set('id_prefix', e.currentTarget.value)}
            />
            <Text size="xs" c="dimmed">
              {t('kong.set.example')} <Code>{exampleId}</Code>
            </Text>
          </Stack>
        </Card>

        <Card>
          <Title order={4} mb="md">
            {t('kong.set.behavior')}
          </Title>
          <Stack>
            <Choice
              label={t('kong.set.routes_created')}
              description={t('kong.set.routes_created_hint')}
              value={draft.route_status}
              onChange={(v) => set('route_status', v)}
              data={[
                { value: 'disabled', label: t('kong.set.disabled') },
                { value: 'enabled', label: t('kong.set.enabled') },
              ]}
            />
            <Choice
              label={t('kong.set.same_id')}
              description={t('kong.set.same_id_hint')}
              value={draft.on_conflict}
              onChange={(v) => set('on_conflict', v)}
              data={[
                { value: 'skip', label: t('kong.set.skip') },
                { value: 'overwrite', label: t('kong.set.overwrite') },
              ]}
            />
          </Stack>
        </Card>

        <Card>
          <Title order={4} mb="md">
            {t('kong.set.plugins')}
          </Title>
          <Stack>
            <Choice
              label={t('kong.set.untranslated')}
              description={t('kong.set.untranslated_hint')}
              value={draft.unsupported_plugins}
              onChange={(v) => set('unsupported_plugins', v)}
              data={[
                { value: 'block', label: t('kong.set.block_object') },
                { value: 'skip', label: t('kong.set.migrate_without') },
              ]}
            />
            <Choice
              label={t('kong.set.disabled_plugin')}
              description={t('kong.set.disabled_plugin_hint')}
              value={draft.disabled_plugins}
              onChange={(v) => set('disabled_plugins', v)}
              data={[
                { value: 'skip', label: t('kong.set.ignore') },
                { value: 'keep', label: t('kong.set.migrate_disabled') },
              ]}
            />
            <Switch
              label={t('kong.set.websocket')}
              description={t('kong.set.websocket_hint')}
              checked={draft.enable_websocket}
              onChange={(e) => set('enable_websocket', e.currentTarget.checked)}
            />
          </Stack>
        </Card>

        <Card>
          <Title order={4} mb={4}>
            {t('kong.set.host_rewrites')}
          </Title>
          <Text size="xs" c="dimmed" mb="md">
            {t('kong.set.host_rewrites_hint')}
          </Text>
          <Stack gap={6}>
            {draft.host_rewrites.map((h, i) => (
              // oxlint-disable-next-line react/no-array-index-key -- rows have no identity and every input is controlled
              <Group key={i} gap="xs" wrap="nowrap">
                <TextInput
                  placeholder="payments.internal"
                  value={h.from}
                  style={{ flex: 1 }}
                  onChange={(e) =>
                    set(
                      'host_rewrites',
                      draft.host_rewrites.map((r, j) => (j === i ? { ...r, from: e.currentTarget.value.trim() } : r)),
                    )
                  }
                />
                <IconArrowRight size={16} style={{ flexShrink: 0 }} />
                <TextInput
                  placeholder="payments_api:8080"
                  value={h.to}
                  style={{ flex: 1 }}
                  onChange={(e) =>
                    set(
                      'host_rewrites',
                      draft.host_rewrites.map((r, j) => (j === i ? { ...r, to: e.currentTarget.value.trim() } : r)),
                    )
                  }
                />
                <ActionIcon
                  variant="subtle"
                  color="red"
                  onClick={() =>
                    set(
                      'host_rewrites',
                      draft.host_rewrites.filter((_, j) => j !== i),
                    )
                  }
                  aria-label={t('common.remove')}
                >
                  <IconTrash size={16} />
                </ActionIcon>
              </Group>
            ))}
            <Button
              size="xs"
              variant="light"
              w="fit-content"
              leftSection={<IconPlus size={14} />}
              onClick={() => set('host_rewrites', [...draft.host_rewrites, { from: '', to: '' }])}
            >
              {t('common.add')}
            </Button>
          </Stack>
          <Title order={4} mt="lg" mb={4}>
            {t('kong.set.extra_labels')}
          </Title>
          <KeyValueEditor
            key={version}
            description={
              <>
                <Trans i18nKey="kong.set.extra_labels_hint" components={{ code: <Code /> }} />
              </>
            }
            value={draft.labels}
            onChange={(v) => set('labels', stringRecord(v))}
            keyPlaceholder={t('kong.set.label_key')}
            valuePlaceholder={t('kong.set.label_value')}
          />
        </Card>
      </SimpleGrid>

      <Card>
        <Group justify="space-between" align="flex-start" mb="md">
          <div>
            <Title order={4}>{t('kong.set.rules')}</Title>
            <Text size="sm" c="dimmed" maw={760}>
              {t('kong.set.rules_hint')}
            </Text>
          </div>
          <Button
            variant="light"
            leftSection={<IconPlus size={16} />}
            onClick={() => {
              set('plugin_rules', [
                ...draft.plugin_rules,
                { kong_plugin: '', action: 'map', apisix_plugin: '', template: undefined },
              ])
              setRuleKeys((k) => [...k, nextRuleKey])
              setNextRuleKey((n) => n + 1)
            }}
          >
            {t('kong.set.new_rule')}
          </Button>
        </Group>
        <Alert variant="light" color="indigo" mb="md" title={t('kong.set.template_help_title')}>
          <Text size="sm">
            <Trans i18nKey="kong.set.template_help" components={TEMPLATE_HELP_PARTS} />
          </Text>
        </Alert>
        <Stack>
          {draft.plugin_rules.length === 0 && (
            <Text size="sm" c="dimmed">
              {t('kong.set.no_rules')}
            </Text>
          )}
          {draft.plugin_rules.map((r, i) => (
            <RuleEditor
              key={ruleKeys[i]}
              rule={r}
              kongPlugins={kongPlugins}
              apisixPlugins={apisixPlugins}
              onChange={(nr) =>
                set(
                  'plugin_rules',
                  draft.plugin_rules.map((x, j) => (j === i ? nr : x)),
                )
              }
              onRemove={() => {
                set(
                  'plugin_rules',
                  draft.plugin_rules.filter((_, j) => j !== i),
                )
                setRuleKeys((k) => k.filter((_, j) => j !== i))
              }}
            />
          ))}
        </Stack>
      </Card>

      {error && (
        <Alert
          color="red"
          variant="light"
          icon={<IconAlertCircle size={16} />}
          title={t('common.save_error')}
          style={{ whiteSpace: 'pre-line' }}
        >
          {error}
        </Alert>
      )}
      <Group justify="flex-end">
        {dirty && (
          <Badge color="orange" variant="light">
            {t('kong.set.unsaved')}
          </Badge>
        )}
        <Button variant="default" disabled={!dirty} onClick={() => setDraft(settings)}>
          {t('kong.set.discard')}
        </Button>
        <Button
          leftSection={<IconDeviceFloppy size={16} />}
          disabled={!dirty}
          loading={save.isPending}
          onClick={() => save.mutate()}
        >
          {t('kong.set.save')}
        </Button>
      </Group>
    </Stack>
  )
}
