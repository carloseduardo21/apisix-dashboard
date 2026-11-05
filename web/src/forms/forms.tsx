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
import { AdvancedFields, LabelsField, NameDesc, OneOrMany, ResourceSelect, Section, str, textProps, type FormProps } from './common'
import { UPSTREAM_HANDLED, UpstreamFields } from './UpstreamForm'

const HTTP_METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS', 'CONNECT', 'TRACE', 'PURGE']

function Sections({ initial, children }: { initial: string[]; children: React.ReactNode }) {
  const [open, setOpen] = useState(initial)
  return (
    <Accordion multiple value={open} onChange={setOpen} variant="separated">
      {children}
    </Accordion>
  )
}

function StatusSwitch({ value, onChange, label = 'Ativo' }: { value: Obj; onChange: (v: Obj) => void; label?: string }) {
  return (
    <Switch
      label={label}
      checked={value.status === undefined || value.status === 1}
      onChange={(e) => onChange(setKey(value, 'status', e.currentTarget.checked ? undefined : 0))}
    />
  )
}

type Target = 'upstream_id' | 'upstream' | 'service_id' | 'none'

/** Where a route/service sends traffic: a shared upstream, an inline one or a service. */
function Destination({ value, onChange, allowService, allowNone }: { value: Obj; onChange: (v: Obj) => void; allowService?: boolean; allowNone?: boolean }) {
  const current: Target =
    value.upstream_id !== undefined ? 'upstream_id'
    : value.upstream !== undefined ? 'upstream'
    : allowService && value.service_id !== undefined ? 'service_id'
    : 'none'
  // A new object without plugins most likely points to an existing upstream.
  const [target, setTarget] = useState<Target>(current === 'none' && (!allowNone || !isObj(value.plugins)) ? 'upstream_id' : current)

  const choose = (t: Target) => {
    setTarget(t)
    let out = setKey(setKey(value, 'upstream_id', undefined), 'upstream', undefined)
    if (allowService) out = setKey(out, 'service_id', undefined)
    if (t === 'upstream') out = { ...out, upstream: { type: 'roundrobin', nodes: [{ host: '', port: 80, weight: 1 }] } }
    onChange(out)
  }

  const options = [
    { value: 'upstream_id', label: 'Upstream existente' },
    { value: 'upstream', label: 'Upstream próprio' },
    ...(allowService ? [{ value: 'service_id', label: 'Service' }] : []),
    ...(allowNone ? [{ value: 'none', label: 'Nenhum' }] : []),
  ]

  return (
    <Stack gap="sm">
      <SegmentedControl value={target} onChange={(t) => choose(t as Target)} data={options} w="fit-content" />
      {target === 'upstream_id' && (
        <ResourceSelect resource="upstreams" label="Upstream" required value={value.upstream_id} onChange={(v) => onChange(setKey(value, 'upstream_id', v))} />
      )}
      {target === 'service_id' && (
        <ResourceSelect
          resource="services"
          label="Service"
          required
          description="A route herda upstream e plugins do service; plugins da route têm precedência."
          value={value.service_id}
          onChange={(v) => onChange(setKey(value, 'service_id', v))}
        />
      )}
      {target === 'upstream' && (
        <UpstreamFields embedded value={isObj(value.upstream) ? value.upstream : {}} onChange={(u) => onChange(setKey(value, 'upstream', u))} />
      )}
      {target === 'none' && (
        <Text size="sm" c="dimmed">
          Sem destino: a requisição precisa ser respondida por um plugin (ex.: redirect, mocking, echo).
        </Text>
      )}
    </Stack>
  )
}

// ---- routes ----

function RouteForm({ value, onChange }: FormProps) {
  return (
    <Sections initial={['general', 'match', 'target', 'plugins']}>
      <Section value="general" title="Geral">
        <Stack gap="sm">
          <NameDesc value={value} onChange={onChange} />
          <StatusSwitch value={value} onChange={onChange} />
        </Stack>
      </Section>
      <Section value="match" title="Correspondência" hint="Quais requisições esta route atende">
        <Stack gap="sm">
          <OneOrMany value={value} onChange={onChange} single="uri" many="uris" label="URIs" required placeholder="/api/*" description="Use * no final para prefixo. Enter para adicionar." />
          <OneOrMany value={value} onChange={onChange} single="host" many="hosts" label="Hosts" placeholder="api.exemplo.com.br" description="Vazio = qualquer host. Aceita *.dominio." />
          <MultiSelect
            label="Métodos"
            description="Vazio = todos os métodos"
            data={HTTP_METHODS}
            value={Array.isArray(value.methods) ? (value.methods as string[]) : []}
            onChange={(m) => onChange(setKey(value, 'methods', m))}
            clearable
            searchable
          />
          <SimpleGrid cols={{ base: 1, sm: 2 }}>
            <OneOrMany value={value} onChange={onChange} single="remote_addr" many="remote_addrs" label="IPs de origem" placeholder="10.0.0.0/8" />
            <NumberInput
              label="Prioridade"
              description="Maior vence quando várias routes casam"
              value={typeof value.priority === 'number' ? value.priority : ''}
              onChange={(v) => onChange(setKey(value, 'priority', typeof v === 'number' ? v : undefined))}
            />
          </SimpleGrid>
        </Stack>
      </Section>
      <Section value="target" title="Destino">
        <Destination value={value} onChange={onChange} allowService allowNone />
      </Section>
      <Section value="plugins" title="Plugins">
        <Stack gap="sm">
          <ResourceSelect
            resource="plugin_configs"
            label="Plugin config"
            description="Conjunto reutilizável de plugins aplicado junto com os desta route"
            value={value.plugin_config_id}
            onChange={(v) => onChange(setKey(value, 'plugin_config_id', v))}
          />
          <PluginsEditor value={value.plugins as Obj} onChange={(p) => onChange(setKey(value, 'plugins', p))} />
        </Stack>
      </Section>
      <Section value="advanced" title="Avançado" hint="Labels, timeouts, websocket, vars e demais campos do schema">
        <Stack gap="sm">
          <LabelsField value={value} onChange={onChange} />
          <AdvancedFields
            schemaName="route"
            value={value}
            onChange={onChange}
            handled={['name', 'desc', 'status', 'uri', 'uris', 'host', 'hosts', 'methods', 'remote_addr', 'remote_addrs', 'priority', 'upstream', 'upstream_id', 'service_id', 'plugin_config_id', 'plugins', 'labels']}
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
      <Section value="general" title="Geral">
        <Stack gap="sm">
          <NameDesc value={value} onChange={onChange} />
          <OneOrMany value={value} onChange={onChange} single="host" many="hosts" label="Hosts" placeholder="api.exemplo.com.br" />
          <Switch label="Habilitar WebSocket" checked={value.enable_websocket === true} onChange={(e) => onChange(setKey(value, 'enable_websocket', e.currentTarget.checked || undefined))} />
        </Stack>
      </Section>
      <Section value="target" title="Destino">
        <Destination value={value} onChange={onChange} allowNone />
      </Section>
      <Section value="plugins" title="Plugins">
        <PluginsEditor value={value.plugins as Obj} onChange={(p) => onChange(setKey(value, 'plugins', p))} />
      </Section>
      <Section value="advanced" title="Avançado">
        <Stack gap="sm">
          <LabelsField value={value} onChange={onChange} />
          <AdvancedFields schemaName="service" value={value} onChange={onChange} handled={['name', 'desc', 'hosts', 'host', 'enable_websocket', 'upstream', 'upstream_id', 'plugins', 'labels']} />
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
      <Section value="general" title="Geral">
        <Stack gap="sm">
          <TextInput
            label="Usuário"
            withAsterisk
            disabled={!isNew}
            description={isNew ? 'Letras, números e _ (não pode ser alterado depois)' : undefined}
            {...textProps(value, onChange, 'username')}
          />
          <Textarea label="Descrição" autosize minRows={1} {...textProps(value, onChange, 'desc')} />
          <ResourceSelect resource="consumer_groups" label="Consumer group" value={value.group_id} onChange={(v) => onChange(setKey(value, 'group_id', v))} />
          <LabelsField value={value} onChange={onChange} />
        </Stack>
      </Section>
      <Section value="plugins" title="Plugins" hint="Prefira cadastrar a autenticação em Credenciais; aqui ficam os demais plugins do consumer">
        <PluginsEditor consumer value={value.plugins as Obj} onChange={(p) => onChange(setKey(value, 'plugins', p))} />
      </Section>
    </Sections>
  )
}

// ---- consumer credentials ----

function CredentialForm({ value, onChange }: FormProps) {
  return (
    <Stack>
      <Textarea label="Descrição" autosize minRows={1} maxRows={4} {...textProps(value, onChange, 'desc')} />
      <LabelsField value={value} onChange={onChange} />
      <Text fw={500}>Autenticação</Text>
      <Text size="sm" c="dimmed">
        Somente plugins de autenticação (key-auth, basic-auth, jwt-auth, hmac-auth...). Os segredos são cifrados pelo APISIX e mascarados na dashboard.
      </Text>
      <PluginsEditor consumer authOnly value={value.plugins as Obj} onChange={(p) => onChange({ ...value, plugins: p ?? {} })} />
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
      <PluginsEditor value={value.plugins as Obj} onChange={(p) => onChange({ ...setKey(value, 'plugins', p), plugins: p ?? {} })} />
    </Stack>
  )
}

const ConsumerGroupForm = (p: FormProps) => <PluginsOnlyForm {...p} withName />
const PluginConfigForm = (p: FormProps) => <PluginsOnlyForm {...p} withName />
const GlobalRuleForm = (p: FormProps) => (
  <Stack>
    <Alert color="yellow" icon={<IconInfoCircle size={16} />}>
      Global rules executam em <b>todas</b> as requisições do gateway.
    </Alert>
    <PluginsOnlyForm {...p} />
  </Stack>
)

// ---- SSL ----

function SslForm({ value, onChange }: FormProps) {
  const masked = value.key === '******'
  return (
    <Sections initial={['cert']}>
      <Section value="cert" title="Certificado">
        <Stack gap="sm">
          <OneOrMany value={value} onChange={onChange} single="sni" many="snis" label="SNIs" required placeholder="api.exemplo.com.br" description="Domínios atendidos por este certificado. Aceita *.dominio." />
          <Textarea
            label="Certificado (PEM)"
            withAsterisk
            description="Inclua a cadeia intermediária após o certificado"
            autosize
            minRows={4}
            maxRows={12}
            styles={{ input: { fontFamily: 'var(--mantine-font-family-monospace)', fontSize: 12 } }}
            {...textProps(value, onChange, 'cert')}
          />
          {masked ? (
            <PasswordInput
              label="Chave privada"
              description="A chave atual está protegida e não é exibida. Para trocar, cole a nova chave em PEM."
              value={str(value.key)}
              onChange={(e) => onChange(setKey(value, 'key', e.currentTarget.value))}
            />
          ) : (
            <Textarea
              label="Chave privada (PEM)"
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
              label="Tipo"
              data={[
                { value: 'server', label: 'Servidor (clientes → gateway)' },
                { value: 'client', label: 'Cliente (gateway → upstream)' },
              ]}
              value={str(value.type) || 'server'}
              allowDeselect={false}
              onChange={(v) => onChange(setKey(value, 'type', v === 'server' ? undefined : v ?? undefined))}
            />
            <StatusSwitch value={value} onChange={onChange} />
          </SimpleGrid>
        </Stack>
      </Section>
      <Section value="advanced" title="Avançado" hint="mTLS de clientes, protocolos, certificados adicionais">
        <Stack gap="sm">
          <LabelsField value={value} onChange={onChange} />
          <AdvancedFields schemaName="ssl" value={value} onChange={onChange} handled={['sni', 'snis', 'cert', 'key', 'type', 'status', 'labels']} />
        </Stack>
      </Section>
    </Sections>
  )
}

// ---- stream routes ----

function StreamRouteForm({ value, onChange }: FormProps) {
  return (
    <Sections initial={['match', 'target']}>
      <Section value="match" title="Correspondência">
        <Stack gap="sm">
          <NameDesc value={value} onChange={onChange} />
          <SimpleGrid cols={{ base: 1, sm: 2 }}>
            <NumberInput
              label="Porta do gateway"
              description="Deve estar em stream_proxy no config.yaml"
              value={typeof value.server_port === 'number' ? value.server_port : ''}
              onChange={(v) => onChange(setKey(value, 'server_port', typeof v === 'number' ? v : undefined))}
            />
            <TextInput label="Endereço do gateway" placeholder="0.0.0.0" {...textProps(value, onChange, 'server_addr')} />
            <TextInput label="IP de origem" placeholder="10.0.0.0/8" {...textProps(value, onChange, 'remote_addr')} />
            <TextInput label="SNI" description="Para tráfego TLS" {...textProps(value, onChange, 'sni')} />
          </SimpleGrid>
        </Stack>
      </Section>
      <Section value="target" title="Destino">
        <Destination value={value} onChange={onChange} />
      </Section>
      <Section value="plugins" title="Plugins">
        <PluginsEditor subsystem="stream" value={value.plugins as Obj} onChange={(p) => onChange(setKey(value, 'plugins', p))} />
      </Section>
      <Section value="advanced" title="Avançado">
        <Stack gap="sm">
          <LabelsField value={value} onChange={onChange} />
          <AdvancedFields
            schemaName="stream_route"
            value={value}
            onChange={onChange}
            handled={['name', 'desc', 'server_port', 'server_addr', 'remote_addr', 'sni', 'upstream', 'upstream_id', 'service_id', 'plugins', 'labels']}
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
        label="Conteúdo (.proto)"
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
        Para HashiCorp Vault use o ID no formato <code>vault/1</code>. Outros gerenciadores (aws, gcp) podem ser editados pela aba JSON.
        Referencie segredos nos plugins com <code>$secret://vault/1/caminho/chave</code>.
      </Alert>
      <TextInput label="URI do Vault" withAsterisk placeholder="https://vault.interno:8200" {...textProps(value, onChange, 'uri')} />
      <TextInput label="Prefixo" withAsterisk placeholder="kv/apisix" {...textProps(value, onChange, 'prefix')} />
      <PasswordInput
        label="Token"
        withAsterisk
        description={value.token === '******' ? 'Token atual mascarado. Mantenha para preservar ou digite um novo.' : 'Prefira um token com política somente leitura'}
        {...textProps(value, onChange, 'token')}
      />
      <TextInput label="Namespace" description="Vault Enterprise" {...textProps(value, onChange, 'namespace')} />
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
