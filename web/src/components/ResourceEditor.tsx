import { useState } from 'react'
import {
  Alert,
  Button,
  Group,
  JsonInput,
  Modal,
  ScrollArea,
  Stack,
  Tabs,
  Text,
  TextInput,
  ThemeIcon,
} from '@mantine/core'
import { IconAlertCircle, IconCode, IconForms, IconInfoCircle } from '@tabler/icons-react'
import { FORMS } from '../forms/forms'
import type { Obj } from '../schema/types'
import type { ResourceDef } from '../resources'
import { errorMessage } from '../values'

interface Props {
  def: ResourceDef
  opened: boolean
  /** Existing object being edited; undefined creates a new one. */
  editing?: { id: string; value: Obj }
  onClose: () => void
  onSubmit: (value: Obj, id: string) => Promise<void>
}

const MASK_RE = /"\*{6}"/

/** Form + JSON editor; both tabs edit the same object. */
export function ResourceEditor({ def, opened, editing, onClose, onSubmit }: Props) {
  const Form = FORMS[def.key]
  const isNew = !editing
  const [draft, setDraft] = useState<Obj>({})
  const [tab, setTab] = useState<string>('form')
  const [text, setText] = useState('')
  const [id, setId] = useState('')
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

  // Start a fresh editing session whenever the dialog opens or its target changes
  // (adjusting state on a prop change, no effect needed).
  const [session, setSession] = useState<{ opened: boolean; editing: Props['editing']; def: Props['def'] }>()
  if (session?.opened !== opened || session.editing !== editing || session.def !== def) {
    setSession({ opened, editing, def })
    if (opened) {
      const initial = editing ? editing.value : structuredClone(def.template)
      setDraft(initial)
      setText(JSON.stringify(initial, null, 2))
      setTab(Form ? 'form' : 'json')
      setId('')
      setError('')
    }
  }

  /** Parses the JSON tab; returns undefined (and shows the error) when invalid. */
  const parseText = (): Obj | undefined => {
    try {
      const v = JSON.parse(text)
      if (!v || typeof v !== 'object' || Array.isArray(v)) throw new Error('o conteúdo deve ser um objeto')
      return v
    } catch (e) {
      setError(`JSON inválido: ${errorMessage(e)}`)
      return undefined
    }
  }

  const switchTab = (next: string | null) => {
    if (!next || next === tab) return
    setError('')
    if (next === 'json') {
      setText(JSON.stringify(draft, null, 2))
    } else {
      const v = parseText()
      if (!v) return
      setDraft(v)
    }
    setTab(next)
  }

  const idMode = def.create === 'put-id' ? 'required' : def.create === 'post' ? 'optional' : 'none'

  const submit = async () => {
    setError('')
    const value = tab === 'json' ? parseText() : draft
    if (!value) return
    if (isNew && idMode === 'required' && !id.trim()) {
      setError('Informe o ID')
      return
    }
    setSaving(true)
    try {
      await onSubmit(value, editing ? editing.id : id.trim())
      onClose()
    } catch (e) {
      setError(errorMessage(e))
    } finally {
      setSaving(false)
    }
  }

  const hasMasked = MASK_RE.test(tab === 'json' ? text : JSON.stringify(draft))

  return (
    <Modal
      opened={opened}
      onClose={onClose}
      size="75rem"
      title={
        <Group gap="sm">
          <ThemeIcon variant="light" size={36}>
            <def.icon size={20} stroke={1.6} />
          </ThemeIcon>
          <div>
            <Text fw={700}>
              {editing ? `Editar ${def.singular}` : `${def.feminine ? 'Nova' : 'Novo'} ${def.singular}`}
            </Text>
            <Text size="xs" c="dimmed" ff={editing ? 'monospace' : undefined}>
              {editing ? editing.id : def.description}
            </Text>
          </div>
        </Group>
      }
      scrollAreaComponent={ScrollArea.Autosize}
      closeOnClickOutside={false}
    >
      <Stack>
        {isNew && idMode !== 'none' && (
          <TextInput
            label="ID"
            withAsterisk={idMode === 'required'}
            description={
              idMode === 'optional'
                ? 'Opcional: deixe vazio para o APISIX gerar'
                : def.key === 'secrets'
                  ? 'Formato gerenciador/id, ex.: vault/1'
                  : def.key === 'credentials'
                    ? 'Letras, números, ".", "_" ou "-", ex.: app-pagamentos'
                    : undefined
            }
            value={id}
            onChange={(e) => setId(e.currentTarget.value)}
            w={360}
          />
        )}

        <Tabs value={tab} onChange={switchTab} keepMounted={false}>
          <Tabs.List>
            {Form && (
              <Tabs.Tab value="form" leftSection={<IconForms size={16} />}>
                Formulário
              </Tabs.Tab>
            )}
            <Tabs.Tab value="json" leftSection={<IconCode size={16} />}>
              JSON
            </Tabs.Tab>
          </Tabs.List>
          {Form && (
            <Tabs.Panel value="form" pt="md">
              <Form value={draft} onChange={setDraft} isNew={isNew} />
            </Tabs.Panel>
          )}
          <Tabs.Panel value="json" pt="md">
            <JsonInput
              value={text}
              onChange={setText}
              autosize
              minRows={16}
              maxRows={32}
              formatOnBlur
              validationError="JSON inválido"
              styles={{ input: { fontFamily: 'var(--mantine-font-family-monospace)', fontSize: 13 } }}
            />
          </Tabs.Panel>
        </Tabs>

        {hasMasked && (
          <Alert color="blue" icon={<IconInfoCircle size={16} />} variant="light">
            Valores <code>******</code> são segredos mascarados: mantidos, preservam o valor atual no APISIX.
          </Alert>
        )}
        {error && (
          <Alert color="red" icon={<IconAlertCircle size={16} />} variant="light" style={{ whiteSpace: 'pre-wrap' }}>
            {error}
          </Alert>
        )}
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            Cancelar
          </Button>
          <Button onClick={submit} loading={saving}>
            Salvar
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}
