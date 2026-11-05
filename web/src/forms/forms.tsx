import { useState, type ComponentType } from 'react'
import {
  Accordion,
  Alert,
  MultiSelect,
  NumberInput,
  PasswordInput,
  SegmentedControl,
  Select,
  SimpleGrid,
  Stack,
  Switch,
  Text,
  TextInput,
  Textarea,
} from '@mantine/core'
import { IconInfoCircle } from '@tabler/icons-react'
import { PluginsEditor } from '../schema/PluginsEditor'
import { isObj, setKey, type Obj } from '../schema/types'
import { asObj } from '../values'
import {
  AdvancedFields,
  LabelsField,
  NameDesc,
  OneOrMany,
  ResourceSelect,
  Section,
  str,
  textProps,
  type FormProps,
} from './common'
import { UPSTREAM_HANDLED, UpstreamFields } from './UpstreamForm'
import { Trans } from 'react-i18next'
import i18n from '../i18n'

const HTTP_METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS', 'CONNECT', 'TRACE', 'PURGE']

function Sections({ initial, children }: { initial: string[]; children: React.ReactNode }) {
  const [open, setOpen] = useState(initial)
  return (
    <Accordion multiple value={open} onChange={setOpen} variant="separated">
      {children}
    </Accordion>
  )
}

function StatusSwitch({ value, onChange, label }: { value: Obj; onChange: (v: Obj) => void; label?: string }) {
  return (
    <Switch
      label={label ?? i18n.t('forms.active')}
      checked={value.status === undefined || value.status === 1}
      onChange={(e) => onChange(setKey(value, 'status', e.currentTarget.checked ? undefined : 0))}
    />
  )
}

const TARGETS = ['upstream_id', 'upstream', 'service_id', 'none'] as const
type Target = (typeof TARGETS)[number]
const isTarget = (t: string): t is Target => (TARGETS as readonly string[]).includes(t)

/** Where a route/service sends traffic: a shared upstream, an inline one or a service. */
function Destination({
  value,
  onChange,
  allowService,
  allowNone,
}: {
  value: Obj
  onChange: (v: Obj) => void
  allowService?: boolean
  allowNone?: boolean
}) {
  const current: Target =
    value.upstream_id !== undefined
      ? 'upstream_id'
      : value.upstream !== undefined
        ? 'upstream'
        : allowService && value.service_id !== undefined
          ? 'service_id'
          : 'none'
  // A new object without plugins most likely points to an existing upstream.
  const [target, setTarget] = useState<Target>(
    current === 'none' && (!allowNone || !isObj(value.plugins)) ? 'upstream_id' : current,
  )

  const choose = (t: Target) => {
    setTarget(t)
    let out = setKey(setKey(value, 'upstream_id', undefined), 'upstream', undefined)
    if (allowService) out = setKey(out, 'service_id', undefined)
    if (t === 'upstream') out = { ...out, upstream: { type: 'roundrobin', nodes: [{ host: '', port: 80, weight: 1 }] } }
    onChange(out)
  }

  const options = [
    { value: 'upstream_id', label: i18n.t('forms.existing_upstream') },
    { value: 'upstream', label: i18n.t('forms.own_upstream') },
    ...(allowService ? [{ value: 'service_id', label: i18n.t('forms.service') }] : []),
    ...(allowNone ? [{ value: 'none', label: i18n.t('forms.none') }] : []),
  ]

  return (
    <Stack gap="sm">
      <SegmentedControl value={target} onChange={(t) => isTarget(t) && choose(t)} data={options} w="fit-content" />
      {target === 'upstream_id' && (
        <ResourceSelect
          resource="upstreams"
          label={i18n.t('forms.upstream')}
          required
          value={value.upstream_id}
          onChange={(v) => onChange(setKey(value, 'upstream_id', v))}
        />
      )}
      {target === 'service_id' && (
        <ResourceSelect
          resource="services"
          label={i18n.t('forms.service')}
          required
          description={i18n.t('forms.service_inherit')}
          value={value.service_id}
          onChange={(v) => onChange(setKey(value, 'service_id', v))}
        />
      )}
      {target === 'upstream' && (
        <UpstreamFields
          embedded
          value={isObj(value.upstream) ? value.upstream : {}}
          onChange={(u) => onChange(setKey(value, 'upstream', u))}
        />
      )}
      {target === 'none' && (
        <Text size="sm" c="dimmed">
          {i18n.t('forms.no_target')}
        </Text>
      )}
    </Stack>
  )
}

// ---- routes ----

function RouteForm({ value, onChange }: FormProps) {
  return (
    <Sections initial={['general', 'match', 'target', 'plugins']}>
      <Section value="general" title={i18n.t('forms.sec_general')}>
        <Stack gap="sm">
          <NameDesc value={value} onChange={onChange} />
          <StatusSwitch value={value} onChange={onChange} />
        </Stack>
      </Section>
      <Section value="match" title={i18n.t('forms.sec_match')} hint={i18n.t('forms.sec_match_hint')}>
        <Stack gap="sm">
          <OneOrMany
            value={value}
            onChange={onChange}
            single="uri"
            many="uris"
            label={i18n.t('forms.uris')}
            required
            placeholder="/api/*"
            description={i18n.t('forms.uris_hint')}
          />
          <OneOrMany
            value={value}
            onChange={onChange}
            single="host"
            many="hosts"
            label={i18n.t('forms.hosts')}
            placeholder={i18n.t('forms.hosts_placeholder')}
            description={i18n.t('forms.hosts_hint')}
          />
          <MultiSelect
            label={i18n.t('forms.methods')}
            description={i18n.t('forms.methods_hint')}
            data={HTTP_METHODS}
            value={Array.isArray(value.methods) ? value.methods.filter((m): m is string => typeof m === 'string') : []}
            onChange={(m) => onChange(setKey(value, 'methods', m))}
            clearable
            searchable
          />
          <SimpleGrid cols={{ base: 1, sm: 2 }}>
            <OneOrMany
              value={value}
              onChange={onChange}
              single="remote_addr"
              many="remote_addrs"
              label={i18n.t('forms.source_ips')}
              placeholder="10.0.0.0/8"
            />
            <NumberInput
              label={i18n.t('forms.priority')}
              description={i18n.t('forms.priority_hint')}
              value={typeof value.priority === 'number' ? value.priority : ''}
              onChange={(v) => onChange(setKey(value, 'priority', typeof v === 'number' ? v : undefined))}
            />
          </SimpleGrid>
        </Stack>
      </Section>
      <Section value="target" title={i18n.t('forms.sec_target')}>
        <Destination value={value} onChange={onChange} allowService allowNone />
      </Section>
      <Section value="plugins" title={i18n.t('forms.sec_plugins')}>
        <Stack gap="sm">
          <ResourceSelect
            resource="plugin_configs"
            label={i18n.t('forms.plugin_config')}
            description={i18n.t('forms.plugin_config_hint')}
            value={value.plugin_config_id}
            onChange={(v) => onChange(setKey(value, 'plugin_config_id', v))}
          />
          <PluginsEditor value={asObj(value.plugins)} onChange={(p) => onChange(setKey(value, 'plugins', p))} />
        </Stack>
      </Section>
      <Section value="advanced" title={i18n.t('forms.sec_advanced')} hint={i18n.t('forms.sec_advanced_hint')}>
        <Stack gap="sm">
          <LabelsField value={value} onChange={onChange} />
          <AdvancedFields
            schemaName="route"
            value={value}
            onChange={onChange}
            handled={[
              'name',
              'desc',
              'status',
              'uri',
              'uris',
              'host',
              'hosts',
              'methods',
              'remote_addr',
              'remote_addrs',
              'priority',
              'upstream',
              'upstream_id',
              'service_id',
              'plugin_config_id',
              'plugins',
              'labels',
            ]}
          />
        </Stack>
      </Section>
    </Sections>
  )
}

// ---- services ----

function ServiceForm({ value, onChange }: FormProps) {
  return (
    <Sections initial={['general', 'target', 'plugins']}>
      <Section value="general" title={i18n.t('forms.sec_general')}>
        <Stack gap="sm">
          <NameDesc value={value} onChange={onChange} />
          <OneOrMany
            value={value}
            onChange={onChange}
            single="host"
            many="hosts"
            label={i18n.t('forms.hosts')}
            placeholder={i18n.t('forms.hosts_placeholder')}
          />
          <Switch
            label={i18n.t('forms.enable_websocket')}
            checked={value.enable_websocket === true}
            onChange={(e) => onChange(setKey(value, 'enable_websocket', e.currentTarget.checked || undefined))}
          />
        </Stack>
      </Section>
      <Section value="target" title={i18n.t('forms.sec_target')}>
        <Destination value={value} onChange={onChange} allowNone />
      </Section>
      <Section value="plugins" title={i18n.t('forms.sec_plugins')}>
        <PluginsEditor value={asObj(value.plugins)} onChange={(p) => onChange(setKey(value, 'plugins', p))} />
      </Section>
      <Section value="advanced" title={i18n.t('forms.sec_advanced')}>
        <Stack gap="sm">
          <LabelsField value={value} onChange={onChange} />
          <AdvancedFields
            schemaName="service"
            value={value}
            onChange={onChange}
            handled={[
              'name',
              'desc',
              'hosts',
              'host',
              'enable_websocket',
              'upstream',
              'upstream_id',
              'plugins',
              'labels',
            ]}
          />
        </Stack>
      </Section>
    </Sections>
  )
}

// ---- upstreams ----

function UpstreamForm({ value, onChange }: FormProps) {
  return (
    <Stack>
      <UpstreamFields value={value} onChange={onChange} />
      <AdvancedFields schemaName="upstream" value={value} onChange={onChange} handled={UPSTREAM_HANDLED} />
    </Stack>
  )
}

// ---- consumers ----

function ConsumerForm({ value, onChange, isNew }: FormProps) {
  return (
    <Sections initial={['general', 'plugins']}>
      <Section value="general" title={i18n.t('forms.sec_general')}>
        <Stack gap="sm">
          <TextInput
            label={i18n.t('forms.username')}
            withAsterisk
            disabled={!isNew}
            description={isNew ? i18n.t('forms.username_hint') : undefined}
            {...textProps(value, onChange, 'username')}
          />
          <Textarea label={i18n.t('common.description')} autosize minRows={1} {...textProps(value, onChange, 'desc')} />
          <ResourceSelect
            resource="consumer_groups"
            label={i18n.t('forms.consumer_group')}
            value={value.group_id}
            onChange={(v) => onChange(setKey(value, 'group_id', v))}
          />
          <LabelsField value={value} onChange={onChange} />
        </Stack>
      </Section>
      <Section value="plugins" title={i18n.t('forms.sec_plugins')} hint={i18n.t('forms.consumer_plugins_hint')}>
        <PluginsEditor consumer value={asObj(value.plugins)} onChange={(p) => onChange(setKey(value, 'plugins', p))} />
      </Section>
    </Sections>
  )
}

// ---- consumer credentials ----

function CredentialForm({ value, onChange }: FormProps) {
  return (
    <Stack>
      <Textarea
        label={i18n.t('common.description')}
        autosize
        minRows={1}
        maxRows={4}
        {...textProps(value, onChange, 'desc')}
      />
      <LabelsField value={value} onChange={onChange} />
      <Text fw={500}>{i18n.t('forms.authentication')}</Text>
      <Text size="sm" c="dimmed">
        {i18n.t('forms.auth_only_hint')}
      </Text>
      <PluginsEditor
        consumer
        authOnly
        value={asObj(value.plugins)}
        onChange={(p) => onChange({ ...value, plugins: p ?? {} })}
      />
    </Stack>
  )
}

// ---- plugin-only resources ----

function PluginsOnlyForm({ value, onChange, withName }: FormProps & { withName?: boolean }) {
  return (
    <Stack>
      {withName && <NameDesc value={value} onChange={onChange} />}
      {withName && <LabelsField value={value} onChange={onChange} />}
      <Text fw={500}>Plugins</Text>
      <PluginsEditor
        value={asObj(value.plugins)}
        onChange={(p) => onChange({ ...setKey(value, 'plugins', p), plugins: p ?? {} })}
      />
    </Stack>
  )
}

const ConsumerGroupForm = (p: FormProps) => <PluginsOnlyForm {...p} withName />
const PluginConfigForm = (p: FormProps) => <PluginsOnlyForm {...p} withName />
const GlobalRuleForm = (p: FormProps) => (
  <Stack>
    <Alert color="yellow" icon={<IconInfoCircle size={16} />}>
      <Trans i18nKey="forms.global_rules_warning" components={{ b: <b /> }} />
    </Alert>
    <PluginsOnlyForm {...p} />
  </Stack>
)

// ---- SSL ----

function SslForm({ value, onChange }: FormProps) {
  const masked = value.key === '******'
  return (
    <Sections initial={['cert']}>
      <Section value="cert" title={i18n.t('forms.sec_certificate')}>
        <Stack gap="sm">
          <OneOrMany
            value={value}
            onChange={onChange}
            single="sni"
            many="snis"
            label={i18n.t('columns.snis')}
            required
            placeholder={i18n.t('forms.hosts_placeholder')}
            description={i18n.t('forms.snis_hint')}
          />
          <Textarea
            label={i18n.t('forms.cert_pem')}
            withAsterisk
            description={i18n.t('forms.cert_pem_hint')}
            autosize
            minRows={4}
            maxRows={12}
            styles={{ input: { fontFamily: 'var(--mantine-font-family-monospace)', fontSize: 12 } }}
            {...textProps(value, onChange, 'cert')}
          />
          {masked ? (
            <PasswordInput
              label={i18n.t('forms.private_key')}
              description={i18n.t('forms.private_key_masked')}
              value={str(value.key)}
              onChange={(e) => onChange(setKey(value, 'key', e.currentTarget.value))}
            />
          ) : (
            <Textarea
              label={i18n.t('forms.private_key_pem')}
              withAsterisk
              autosize
              minRows={4}
              maxRows={12}
              styles={{ input: { fontFamily: 'var(--mantine-font-family-monospace)', fontSize: 12 } }}
              {...textProps(value, onChange, 'key')}
            />
          )}
          <SimpleGrid cols={{ base: 1, sm: 2 }}>
            <Select
              label={i18n.t('forms.type')}
              data={[
                { value: 'server', label: i18n.t('forms.ssl_server') },
                { value: 'client', label: i18n.t('forms.ssl_client') },
              ]}
              value={str(value.type) || 'server'}
              allowDeselect={false}
              onChange={(v) => onChange(setKey(value, 'type', v === 'server' ? undefined : (v ?? undefined)))}
            />
            <StatusSwitch value={value} onChange={onChange} />
          </SimpleGrid>
        </Stack>
      </Section>
      <Section value="advanced" title={i18n.t('forms.sec_advanced')} hint={i18n.t('forms.ssl_advanced_hint')}>
        <Stack gap="sm">
          <LabelsField value={value} onChange={onChange} />
          <AdvancedFields
            schemaName="ssl"
            value={value}
            onChange={onChange}
            handled={['sni', 'snis', 'cert', 'key', 'type', 'status', 'labels']}
          />
        </Stack>
      </Section>
    </Sections>
  )
}

// ---- stream routes ----

function StreamRouteForm({ value, onChange }: FormProps) {
  return (
    <Sections initial={['match', 'target']}>
      <Section value="match" title={i18n.t('forms.sec_match')}>
        <Stack gap="sm">
          <NameDesc value={value} onChange={onChange} />
          <SimpleGrid cols={{ base: 1, sm: 2 }}>
            <NumberInput
              label={i18n.t('forms.gateway_port')}
              description={i18n.t('forms.gateway_port_hint')}
              value={typeof value.server_port === 'number' ? value.server_port : ''}
              onChange={(v) => onChange(setKey(value, 'server_port', typeof v === 'number' ? v : undefined))}
            />
            <TextInput
              label={i18n.t('forms.gateway_addr')}
              placeholder="0.0.0.0"
              {...textProps(value, onChange, 'server_addr')}
            />
            <TextInput
              label={i18n.t('forms.source_ip')}
              placeholder="10.0.0.0/8"
              {...textProps(value, onChange, 'remote_addr')}
            />
            <TextInput label="SNI" description={i18n.t('forms.sni_hint')} {...textProps(value, onChange, 'sni')} />
          </SimpleGrid>
        </Stack>
      </Section>
      <Section value="target" title={i18n.t('forms.sec_target')}>
        <Destination value={value} onChange={onChange} />
      </Section>
      <Section value="plugins" title={i18n.t('forms.sec_plugins')}>
        <PluginsEditor
          subsystem="stream"
          value={asObj(value.plugins)}
          onChange={(p) => onChange(setKey(value, 'plugins', p))}
        />
      </Section>
      <Section value="advanced" title={i18n.t('forms.sec_advanced')}>
        <Stack gap="sm">
          <LabelsField value={value} onChange={onChange} />
          <AdvancedFields
            schemaName="stream_route"
            value={value}
            onChange={onChange}
            handled={[
              'name',
              'desc',
              'server_port',
              'server_addr',
              'remote_addr',
              'sni',
              'upstream',
              'upstream_id',
              'service_id',
              'plugins',
              'labels',
            ]}
          />
        </Stack>
      </Section>
    </Sections>
  )
}

// ---- protos ----

function ProtoForm({ value, onChange }: FormProps) {
  return (
    <Stack>
      <NameDesc value={value} onChange={onChange} />
      <Textarea
        label={i18n.t('forms.proto_content')}
        withAsterisk
        autosize
        minRows={10}
        maxRows={30}
        styles={{ input: { fontFamily: 'var(--mantine-font-family-monospace)', fontSize: 12 } }}
        {...textProps(value, onChange, 'content')}
      />
      <LabelsField value={value} onChange={onChange} />
    </Stack>
  )
}

// ---- secrets (Vault) ----

function SecretForm({ value, onChange }: FormProps) {
  return (
    <Stack>
      <Alert color="blue" icon={<IconInfoCircle size={16} />}>
        <Trans i18nKey="forms.vault_hint" components={{ code: <code /> }} />
      </Alert>
      <TextInput
        label={i18n.t('forms.vault_uri')}
        withAsterisk
        placeholder="https://vault.internal:8200"
        {...textProps(value, onChange, 'uri')}
      />
      <TextInput
        label={i18n.t('forms.prefix')}
        withAsterisk
        placeholder="kv/apisix"
        {...textProps(value, onChange, 'prefix')}
      />
      <PasswordInput
        label={i18n.t('forms.token')}
        withAsterisk
        description={value.token === '******' ? i18n.t('forms.token_masked') : i18n.t('forms.token_hint')}
        {...textProps(value, onChange, 'token')}
      />
      <TextInput
        label={i18n.t('forms.namespace')}
        description="Vault Enterprise"
        {...textProps(value, onChange, 'namespace')}
      />
    </Stack>
  )
}

export const FORMS: Record<string, ComponentType<FormProps>> = {
  routes: RouteForm,
  services: ServiceForm,
  upstreams: UpstreamForm,
  consumers: ConsumerForm,
  credentials: CredentialForm,
  consumer_groups: ConsumerGroupForm,
  plugin_configs: PluginConfigForm,
  global_rules: GlobalRuleForm,
  ssls: SslForm,
  stream_routes: StreamRouteForm,
  protos: ProtoForm,
  secrets: SecretForm,
}
