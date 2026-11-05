import { Accordion, ActionIcon, Button, Group, Input, NumberInput, SegmentedControl, Select, SimpleGrid, Stack, Text, TextInput } from '@mantine/core'
import { IconPlus, IconTrash } from '@tabler/icons-react'
import { ObjectFields, SchemaField } from '../schema/SchemaForm'
import { isObj, setKey, useCatalog, type Obj } from '../schema/types'
import { LabelsField, NameDesc, str, textProps } from './common'

interface Node {
  host: string
  port?: number
  weight: number
  priority?: number
}

/** APISIX accepts {"host:port": weight} or [{host, port, weight}]; the form edits the list. */
function toNodes(v: unknown): Node[] {
  if (Array.isArray(v)) return v.filter(isObj).map((n) => ({ ...(n as unknown as Node) }))
  if (isObj(v)) {
    return Object.entries(v).map(([addr, weight]) => {
      const i = addr.lastIndexOf(':')
      return i > 0 ? { host: addr.slice(0, i), port: Number(addr.slice(i + 1)), weight: Number(weight) } : { host: addr, weight: Number(weight) }
    })
  }
  return []
}

function NodesEditor({ value, onChange }: { value: unknown; onChange: (v: Node[]) => void }) {
  const nodes = toNodes(value)
  const set = (i: number, patch: Partial<Node>) => onChange(nodes.map((n, j) => (j === i ? { ...n, ...patch } : n)))
  return (
    <Input.Wrapper label="Nós" withAsterisk description="Destinos do balanceamento. Em Swarm, use o nome do serviço (resolvido pelo DNS interno).">
      <Stack gap={6} mt={4}>
        {nodes.length > 0 && (
          <Group gap="xs" wrap="nowrap">
            <Text size="xs" c="dimmed" style={{ flex: 3 }}>Host</Text>
            <Text size="xs" c="dimmed" style={{ flex: 1 }}>Porta</Text>
            <Text size="xs" c="dimmed" style={{ flex: 1 }}>Peso</Text>
            <Text size="xs" c="dimmed" style={{ flex: 1 }}>Prioridade</Text>
            <div style={{ width: 28 }} />
          </Group>
        )}
        {nodes.map((n, i) => (
          <Group key={i} gap="xs" wrap="nowrap">
            <TextInput style={{ flex: 3 }} value={n.host ?? ''} placeholder="servico" onChange={(e) => set(i, { host: e.currentTarget.value })} />
            <NumberInput style={{ flex: 1 }} value={n.port ?? ''} min={1} max={65535} placeholder="80" onChange={(v) => set(i, { port: typeof v === 'number' ? v : undefined })} />
            <NumberInput style={{ flex: 1 }} value={n.weight ?? 1} min={0} onChange={(v) => set(i, { weight: typeof v === 'number' ? v : 0 })} />
            <NumberInput style={{ flex: 1 }} value={n.priority ?? 0} onChange={(v) => set(i, { priority: typeof v === 'number' && v !== 0 ? v : undefined })} />
            <ActionIcon variant="subtle" color="red" onClick={() => onChange(nodes.filter((_, j) => j !== i))}>
              <IconTrash size={16} />
            </ActionIcon>
          </Group>
        ))}
        <Button size="xs" variant="light" leftSection={<IconPlus size={14} />} w="fit-content" onClick={() => onChange([...nodes, { host: '', port: 80, weight: 1 }])}>
          Adicionar nó
        </Button>
      </Stack>
    </Input.Wrapper>
  )
}

const LB_TYPES = [
  { value: 'roundrobin', label: 'Round robin (ponderado)' },
  { value: 'chash', label: 'Hash consistente' },
  { value: 'least_conn', label: 'Menos conexões' },
  { value: 'ewma', label: 'Menor latência (EWMA)' },
]

// [fields, title, accordion id, hint]
const OPTIONAL_GROUPS: [string[], string, string, string][] = [
  [['timeout'], 'Timeouts', 'timeout', 'connect, send e read em segundos (se definir, informe os três)'],
  [['retries', 'retry_timeout'], 'Retentativas', 'retries', 'Quantas vezes tentar outro nó em caso de falha'],
  [['checks'], 'Health checks', 'checks', 'Verificações ativas e passivas dos nós'],
  [['tls'], 'TLS com o upstream', 'tls', 'Certificado de cliente (mTLS) e verificação do servidor'],
  [['keepalive_pool'], 'Pool de keepalive', 'keepalive', 'Conexões reutilizadas com os nós'],
]

const SCHEMES = ['http', 'https', 'grpc', 'grpcs', 'ws', 'wss', 'tcp', 'tls', 'udp', 'kafka']

/**
 * Upstream fields. Used by the upstream resource and for inline upstreams
 * inside routes/services (`embedded` hides name/labels).
 */
export function UpstreamFields({ value, onChange, embedded }: { value: Obj; onChange: (v: Obj) => void; embedded?: boolean }) {
  const catalog = useCatalog()
  const props = catalog.data?.resources.upstream?.properties ?? {}
  const discovery = value.service_name !== undefined || value.discovery_type !== undefined
  const type = str(value.type) || 'roundrobin'
  const sub = (key: string, label: string) =>
    props[key] && (
      <SchemaField
        name={label}
        schema={props[key]}
        value={value[key]}
        path={key}
        onChange={(v) => onChange(setKey(value, key, v))}
      />
    )

  return (
    <Stack gap="sm">
      {!embedded && <NameDesc value={value} onChange={onChange} />}

      <SimpleGrid cols={{ base: 1, sm: 2 }}>
        <Select
          label="Balanceamento"
          data={LB_TYPES}
          value={type}
          allowDeselect={false}
          onChange={(v) => onChange(setKey(setKey(setKey(value, 'type', v ?? undefined), 'hash_on', undefined), 'key', undefined))}
        />
        <Select label="Protocolo" data={SCHEMES} value={str(value.scheme) || 'http'} allowDeselect={false} onChange={(v) => onChange(setKey(value, 'scheme', v === 'http' ? undefined : v ?? undefined))} />
      </SimpleGrid>

      {type === 'chash' && (
        <SimpleGrid cols={{ base: 1, sm: 2 }}>
          {sub('hash_on', 'Hash com base em')}
          <TextInput label="Chave do hash" withAsterisk description="Ex.: remote_addr, uri, nome do header/cookie" {...textProps(value, onChange, 'key')} />
        </SimpleGrid>
      )}

      <SegmentedControl
        value={discovery ? 'discovery' : 'nodes'}
        onChange={(m) =>
          onChange(
            m === 'nodes'
              ? setKey(setKey(setKey(setKey(value, 'service_name', undefined), 'discovery_type', undefined), 'discovery_args', undefined), 'nodes', [{ host: '', port: 80, weight: 1 }])
              : setKey(setKey(value, 'nodes', undefined), 'discovery_type', 'dns'),
          )
        }
        data={[
          { value: 'nodes', label: 'Nós fixos' },
          { value: 'discovery', label: 'Service discovery' },
        ]}
        w="fit-content"
      />
      {discovery ? (
        <SimpleGrid cols={{ base: 1, sm: 2 }}>
          <TextInput label="Tipo de discovery" withAsterisk description="dns, consul, nacos, eureka, kubernetes..." {...textProps(value, onChange, 'discovery_type')} />
          <TextInput label="Nome do serviço" withAsterisk description="Para DNS: host:porta" {...textProps(value, onChange, 'service_name')} />
        </SimpleGrid>
      ) : (
        <NodesEditor value={value.nodes} onChange={(nodes) => onChange(setKey(value, 'nodes', nodes))} />
      )}

      <SimpleGrid cols={{ base: 1, sm: 2 }}>
        {sub('pass_host', 'Header Host enviado')}
        {value.pass_host === 'rewrite' && <TextInput label="Host reescrito" withAsterisk {...textProps(value, onChange, 'upstream_host')} />}
      </SimpleGrid>
      {/* Optional groups stay collapsed; once a group is set APISIX requires its fields. */}
      <Accordion multiple variant="contained" defaultValue={OPTIONAL_GROUPS.filter(([keys]) => keys.some((k) => value[k] !== undefined)).map(([, , id]) => id)}>
        {OPTIONAL_GROUPS.map(([keys, title, id, hint]) => (
          <Accordion.Item key={id} value={id}>
            <Accordion.Control>
              <Group gap="xs">
                <Text size="sm" fw={500}>{title}</Text>
                {keys.some((k) => value[k] !== undefined) && <Text size="xs" c="teal">configurado</Text>}
              </Group>
              <Text size="xs" c="dimmed">{hint}</Text>
            </Accordion.Control>
            <Accordion.Panel>
              <Stack gap="sm">
                {keys.length === 1 ? (
                  props[keys[0]] && (
                    <ObjectFields
                      schema={props[keys[0]]}
                      value={value[keys[0]]}
                      path={keys[0]}
                      onChange={(v) => onChange(setKey(value, keys[0], v))}
                      moreLabel="Opções"
                    />
                  )
                ) : (
                  <SimpleGrid cols={{ base: 1, sm: 2 }}>
                    {sub('retries', 'Tentativas')}
                    {sub('retry_timeout', 'Tempo máximo de retentativas (s)')}
                  </SimpleGrid>
                )}
              </Stack>
            </Accordion.Panel>
          </Accordion.Item>
        ))}
      </Accordion>
      {!embedded && <LabelsField value={value} onChange={onChange} />}
    </Stack>
  )
}

export const UPSTREAM_HANDLED = [
  'name', 'desc', 'type', 'scheme', 'hash_on', 'key', 'nodes', 'service_name', 'discovery_type', 'discovery_args',
  'pass_host', 'upstream_host', 'timeout', 'retries', 'retry_timeout', 'checks', 'tls', 'keepalive_pool', 'labels',
]
