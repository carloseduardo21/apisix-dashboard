import { useEffect, useMemo, useState } from 'react'
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
      <SegmentedControl fullWidth value={value} onChange={(v) => onChange(v as T)} data={data} />
    </div>
  )
}

const TEMPLATE_EXAMPLE = `{
  "nivel": "\${config.nivel}",
  "limite": "\${config.limite|10}",
  "rota": "\${apisix.route_id}"
}`

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
      const t = JSON.parse(v)
      if (!t || typeof t !== 'object' || Array.isArray(t)) throw new Error('o template deve ser um objeto')
      setError('')
      onChange({ ...rule, template: t })
    } catch (e) {
      setError((e as Error).message)
    }
  }
  return (
    <Card withBorder padding="md">
      <Group align="flex-end" wrap="wrap" gap="sm">
        <Autocomplete
          label="Plugin do Kong"
          placeholder="meu-plugin"
          data={kongPlugins}
          value={rule.kong_plugin}
          onChange={(v) => onChange({ ...rule, kong_plugin: v.trim() })}
          style={{ flex: 1, minWidth: 180 }}
        />
        <SegmentedControl
          value={rule.action}
          onChange={(v) => onChange({ ...rule, action: v as PluginRule['action'] })}
          data={[
            { value: 'map', label: 'Mapear' },
            { value: 'ignore', label: 'Ignorar' },
          ]}
        />
        {rule.action === 'map' && (
          <>
            <IconArrowRight size={18} style={{ marginBottom: 8 }} />
            <Autocomplete
              label="Plugin do APISIX"
              placeholder="meu-plugin-apisix"
              data={apisixPlugins}
              value={rule.apisix_plugin ?? ''}
              onChange={(v) => onChange({ ...rule, apisix_plugin: v.trim() })}
              style={{ flex: 1, minWidth: 180 }}
            />
          </>
        )}
        <ActionIcon variant="subtle" color="red" size="lg" onClick={onRemove} aria-label="Remover regra" mb={2}>
          <IconTrash size={18} />
        </ActionIcon>
      </Group>
      {rule.action === 'map' && (
        <JsonInput
          mt="sm"
          label="Template da configuração"
          description="Objeto JSON gerado para cada ocorrência do plugin. Campos com valor ausente são omitidos."
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
  const qc = useQueryClient()
  const catalog = useCatalog()
  const [draft, setDraft] = useState<Settings>(settings)
  const [error, setError] = useState('')
  // Remounts the key/value editor when the saved settings are reloaded.
  const [version, setVersion] = useState(0)
  useEffect(() => {
    setDraft(settings)
    setVersion((v) => v + 1)
  }, [settings])

  const apisixPlugins = useMemo(() => Object.keys(catalog.data?.plugins ?? {}).sort(), [catalog.data])
  const dirty = JSON.stringify(draft) !== JSON.stringify(settings)
  const set = <K extends keyof Settings>(k: K, v: Settings[K]) => setDraft((d) => ({ ...d, [k]: v }))

  const save = useMutation({
    mutationFn: () => api<Settings>('PUT', '/api/migration/kong/settings', draft),
    onSuccess: () => {
      setError('')
      notifications.show({ color: 'teal', icon: <IconCheck size={18} />, title: 'Padrões salvos', message: 'O inventário foi recalculado com os novos padrões.' })
      qc.invalidateQueries({ queryKey: ['kong'] })
    },
    onError: (e) => setError((e as Error).message),
  })

  const exampleId = `${draft.id_prefix}${draft.id_strategy === 'name' ? 'pagamentos-api' : '5d34560d-8c3c-…'}`

  return (
    <Stack>
      <SimpleGrid cols={{ base: 1, md: 2 }}>
        <Card>
          <Title order={4} mb="md">
            Identificação
          </Title>
          <Stack>
            <Choice
              label="ID no APISIX"
              description="Pelo nome os IDs ficam legíveis; pelo ID do Kong não mudam se algo for renomeado."
              value={draft.id_strategy}
              onChange={(v) => set('id_strategy', v)}
              data={[
                { value: 'name', label: 'Nome do Kong' },
                { value: 'kong_id', label: 'ID do Kong' },
              ]}
            />
            <TextInput
              label="Prefixo dos IDs"
              description="Aplicado a rotas, services, upstreams e plugins globais. Usernames de consumers não mudam."
              placeholder="kong-"
              maxLength={20}
              value={draft.id_prefix}
              onChange={(e) => set('id_prefix', e.currentTarget.value)}
            />
            <Text size="xs" c="dimmed">
              Exemplo: <Code>{exampleId}</Code>
            </Text>
          </Stack>
        </Card>

        <Card>
          <Title order={4} mb="md">
            Comportamento
          </Title>
          <Stack>
            <Choice
              label="Rotas criadas"
              description="Desabilitadas, as rotas só passam a atender quando ativadas no cutover."
              value={draft.route_status}
              onChange={(v) => set('route_status', v)}
              data={[
                { value: 'disabled', label: 'Desabilitadas' },
                { value: 'enabled', label: 'Habilitadas' },
              ]}
            />
            <Choice
              label="Objeto com o mesmo ID no APISIX"
              description="Vale para objetos que não vieram desta migração."
              value={draft.on_conflict}
              onChange={(v) => set('on_conflict', v)}
              data={[
                { value: 'skip', label: 'Pular' },
                { value: 'overwrite', label: 'Sobrescrever' },
              ]}
            />
          </Stack>
        </Card>

        <Card>
          <Title order={4} mb="md">
            Plugins
          </Title>
          <Stack>
            <Choice
              label="Plugin sem tradução"
              description="Bloquear evita publicar uma rota sem, por exemplo, a autenticação."
              value={draft.unsupported_plugins}
              onChange={(v) => set('unsupported_plugins', v)}
              data={[
                { value: 'block', label: 'Bloquear o objeto' },
                { value: 'skip', label: 'Migrar sem o plugin' },
              ]}
            />
            <Choice
              label="Plugin desabilitado no Kong"
              description="Migrar desabilitado usa _meta.disable no APISIX."
              value={draft.disabled_plugins}
              onChange={(v) => set('disabled_plugins', v)}
              data={[
                { value: 'skip', label: 'Ignorar' },
                { value: 'keep', label: 'Migrar desabilitado' },
              ]}
            />
            <Switch
              label="Habilitar WebSocket nas rotas"
              description="O Kong aceita upgrade para WebSocket em qualquer rota HTTP; o APISIX só com enable_websocket."
              checked={draft.enable_websocket}
              onChange={(e) => set('enable_websocket', e.currentTarget.checked)}
            />
          </Stack>
        </Card>

        <Card>
          <Title order={4} mb={4}>
            Reescrita de hosts
          </Title>
          <Text size="xs" c="dimmed" mb="md">
            Troca o host de services e targets, por exemplo pelo nome do serviço no Swarm. O header Host enviado continua o original.
          </Text>
          <Stack gap={6}>
            {draft.host_rewrites.map((h, i) => (
              <Group key={i} gap="xs" wrap="nowrap">
                <TextInput
                  placeholder="pagamentos.interno"
                  value={h.from}
                  style={{ flex: 1 }}
                  onChange={(e) => set('host_rewrites', draft.host_rewrites.map((r, j) => (j === i ? { ...r, from: e.currentTarget.value.trim() } : r)))}
                />
                <IconArrowRight size={16} style={{ flexShrink: 0 }} />
                <TextInput
                  placeholder="pagamentos_api:8080"
                  value={h.to}
                  style={{ flex: 1 }}
                  onChange={(e) => set('host_rewrites', draft.host_rewrites.map((r, j) => (j === i ? { ...r, to: e.currentTarget.value.trim() } : r)))}
                />
                <ActionIcon variant="subtle" color="red" onClick={() => set('host_rewrites', draft.host_rewrites.filter((_, j) => j !== i))} aria-label="Remover">
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
              Adicionar
            </Button>
          </Stack>
          <Title order={4} mt="lg" mb={4}>
            Labels extras
          </Title>
          <KeyValueEditor
            key={version}
            description={
              <>
                Somados a <Code>migrated-from=kong</Code> e <Code>kong-id</Code>, que a migração já grava.
              </>
            }
            value={draft.labels}
            onChange={(v) => set('labels', (v as Record<string, string>) ?? {})}
            keyPlaceholder="lote"
            valuePlaceholder="onda-1"
          />
        </Card>
      </SimpleGrid>

      <Card>
        <Group justify="space-between" align="flex-start" mb="md">
          <div>
            <Title order={4}>Regras de plugins</Title>
            <Text size="sm" c="dimmed" maw={760}>
              Para plugins custom (ou para trocar a tradução padrão). A regra vale para todas as ocorrências do plugin: services, rotas, consumers e globais.
            </Text>
          </div>
          <Button
            variant="light"
            leftSection={<IconPlus size={16} />}
            onClick={() => set('plugin_rules', [...draft.plugin_rules, { kong_plugin: '', action: 'map', apisix_plugin: '', template: undefined }])}
          >
            Nova regra
          </Button>
        </Group>
        <Alert variant="light" color="indigo" mb="md" title="Como escrever o template">
          <Text size="sm">
            <Code>{'${config.campo}'}</Code> copia o valor do plugin no Kong mantendo o tipo; <Code>{'${config.campo|padrão}'}</Code> usa o padrão quando ausente;{' '}
            <Code>{'${config}'}</Code> copia a configuração inteira. Também existem <Code>plugin</Code>, <Code>service</Code>, <Code>route</Code>,{' '}
            <Code>consumer</Code> (objetos do Kong) e <Code>apisix.route_id</Code>, <Code>apisix.service_id</Code>, <Code>apisix.upstream_id</Code>,{' '}
            <Code>apisix.consumer</Code>.
          </Text>
        </Alert>
        <Stack>
          {draft.plugin_rules.length === 0 && (
            <Text size="sm" c="dimmed">
              Nenhuma regra. Plugins sem tradução automática aparecem no inventário como bloqueados.
            </Text>
          )}
          {draft.plugin_rules.map((r, i) => (
            <RuleEditor
              key={`${version}-${i}`}
              rule={r}
              kongPlugins={kongPlugins}
              apisixPlugins={apisixPlugins}
              onChange={(nr) => set('plugin_rules', draft.plugin_rules.map((x, j) => (j === i ? nr : x)))}
              onRemove={() => set('plugin_rules', draft.plugin_rules.filter((_, j) => j !== i))}
            />
          ))}
        </Stack>
      </Card>

      {error && (
        <Alert color="red" variant="light" icon={<IconAlertCircle size={16} />} title="Não foi possível salvar" style={{ whiteSpace: 'pre-line' }}>
          {error}
        </Alert>
      )}
      <Group justify="flex-end">
        {dirty && <Badge color="orange" variant="light">alterações não salvas</Badge>}
        <Button variant="default" disabled={!dirty} onClick={() => setDraft(settings)}>
          Descartar
        </Button>
        <Button leftSection={<IconDeviceFloppy size={16} />} disabled={!dirty} loading={save.isPending} onClick={() => save.mutate()}>
          Salvar padrões
        </Button>
      </Group>
    </Stack>
  )
}
