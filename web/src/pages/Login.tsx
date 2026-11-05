import { useEffect, useState, type FormEvent } from 'react'
import {
  Alert,
  Anchor,
  Box,
  Button,
  Center,
  Code,
  Group,
  Image,
  Paper,
  PasswordInput,
  PinInput,
  Stack,
  Text,
  TextInput,
  ThemeIcon,
  Title,
} from '@mantine/core'
import { IconAlertCircle, IconArrowLeft, IconHistory, IconLock, IconShieldLock } from '@tabler/icons-react'
import { Logo, LogoMark } from '../components/Logo'
import classes from './Login.module.css'
import QRCode from 'qrcode'
import { api, setCsrfToken, type Me } from '../api'
import { useTranslation } from 'react-i18next'
import { LanguageSelect } from '../components/LanguageSelect'

function errText(e: unknown) {
  return e instanceof Error ? e.message : String(e)
}

function Brand() {
  const { t } = useTranslation()
  const features = [
    { icon: IconShieldLock, title: t('login.feature_mfa_title'), text: t('login.feature_mfa') },
    { icon: IconHistory, title: t('login.feature_audit_title'), text: t('login.feature_audit') },
    { icon: IconLock, title: t('login.feature_secrets_title'), text: t('login.feature_secrets') },
  ]
  return (
    <div className={classes.brand}>
      <Group gap="sm">
        <LogoMark size={40} />
        <div>
          <Text fw={800} size="lg" lh={1.1}>
            {t('app.name')}
          </Text>
          <Text size="sm" opacity={0.8} lh={1.1}>
            {t('app.tagline')}
          </Text>
        </div>
      </Group>
      <Stack gap="lg" maw={480}>
        <Title order={1} fz={40} lh={1.15} fw={800}>
          {t('login.headline')}
        </Title>
        <Text size="lg" opacity={0.85}>
          {t('login.subheadline')}
        </Text>
        <Stack gap="sm" mt="md">
          {features.map((f) => (
            <Group key={f.title} className={classes.feature} wrap="nowrap" align="flex-start">
              <ThemeIcon size={38} radius="md" variant="white" color="indigo">
                <f.icon size={20} stroke={1.6} />
              </ThemeIcon>
              <div>
                <Text fw={600}>{f.title}</Text>
                <Text size="sm" opacity={0.8}>
                  {f.text}
                </Text>
              </div>
            </Group>
          ))}
        </Stack>
      </Stack>
      <Text size="xs" opacity={0.6}>
        {t('login.restricted')}
      </Text>
    </div>
  )
}

function Card({ title, subtitle, children }: { title: string; subtitle?: string; children: React.ReactNode }) {
  return (
    <div className={classes.wrapper}>
      <Brand />
      <div className={classes.formSide}>
        <div className={classes.form}>
          <Group justify="space-between" mb="xl" wrap="nowrap">
            <Box hiddenFrom="md">
              <Logo size={36} />
            </Box>
            <Box ml="auto">
              <LanguageSelect />
            </Box>
          </Group>
          <Title order={2} fz={28} fw={700}>
            {title}
          </Title>
          {subtitle && (
            <Text c="dimmed" size="sm" mt={6}>
              {subtitle}
            </Text>
          )}
          <Box mt="xl">{children}</Box>
        </div>
      </div>
    </div>
  )
}

function ErrorAlert({ error }: { error: string }) {
  if (!error) return null
  return (
    <Alert color="red" icon={<IconAlertCircle size={16} />} variant="light">
      {error}
    </Alert>
  )
}

/** Login → MFA (enroll or verify) → mandatory password change. */
export function LoginFlow({ me, onDone }: { me: Me | null; onDone: () => void }) {
  if (!me) return <PasswordStep onDone={onDone} />
  if (!me.mfa_ok) return me.mfa_enrolled ? <MfaVerifyStep onDone={onDone} /> : <MfaEnrollStep me={me} onDone={onDone} />
  return <ChangePasswordStep onDone={onDone} />
}

function PasswordStep({ onDone }: { onDone: () => void }) {
  const { t } = useTranslation()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const res = await api<{ csrf_token: string }>('POST', '/api/auth/login', { username, password })
      setCsrfToken(res.csrf_token)
      onDone()
    } catch (err) {
      setError(errText(err))
      setPassword('')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Card title={t('login.title')} subtitle={t('login.subtitle')}>
      <form onSubmit={submit}>
        <Stack>
          <ErrorAlert error={error} />
          <TextInput
            size="md"
            label={t('login.username')}
            autoComplete="username"
            value={username}
            onChange={(e) => setUsername(e.currentTarget.value)}
            required
            data-autofocus
          />
          <PasswordInput
            size="md"
            label={t('login.password')}
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.currentTarget.value)}
            required
          />
          <Button type="submit" loading={loading} fullWidth size="md" mt="xs" variant="gradient">
            {t('login.continue')}
          </Button>
        </Stack>
      </form>
    </Card>
  )
}

function LogoutLink({ onDone }: { onDone: () => void }) {
  const { t } = useTranslation()
  return (
    <Anchor
      component="button"
      size="sm"
      c="dimmed"
      onClick={async () => {
        await api('POST', '/api/auth/logout').catch(() => undefined)
        onDone()
      }}
    >
      <Group gap={4}>
        <IconArrowLeft size={14} /> {t('login.back')}
      </Group>
    </Anchor>
  )
}

function MfaVerifyStep({ onDone }: { onDone: () => void }) {
  const { t } = useTranslation()
  const [code, setCode] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const verify = async (value: string) => {
    setError('')
    setLoading(true)
    try {
      await api('POST', '/api/auth/mfa/verify', { code: value })
      onDone()
    } catch (err) {
      setError(errText(err))
      setCode('')
      // A lockout ends the session; refreshing sends the user back to login.
      onDone()
    } finally {
      setLoading(false)
    }
  }

  return (
    <Card title={t('login.mfa_title')} subtitle={t('login.mfa_subtitle')}>
      <Stack align="center">
        <ErrorAlert error={error} />
        <PinInput
          size="lg"
          length={6}
          type="number"
          oneTimeCode
          // oxlint-disable-next-line jsx-a11y/no-autofocus -- single-purpose MFA step; focusing the only input is expected
          autoFocus
          value={code}
          onChange={setCode}
          onComplete={verify}
          disabled={loading}
        />
        <Button
          fullWidth
          size="md"
          variant="gradient"
          loading={loading}
          disabled={code.length !== 6}
          onClick={() => verify(code)}
        >
          {t('login.verify')}
        </Button>
        <LogoutLink onDone={onDone} />
      </Stack>
    </Card>
  )
}

function MfaEnrollStep({ me, onDone }: { me: Me; onDone: () => void }) {
  const { t } = useTranslation()
  const [secret, setSecret] = useState<{ secret: string; uri: string } | null>(null)
  const [qr, setQr] = useState('')
  const [code, setCode] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    api<{ secret: string; uri: string }>('POST', '/api/auth/mfa/enroll/start')
      .then(async (s) => {
        setSecret(s)
        setQr(await QRCode.toDataURL(s.uri, { margin: 1, width: 200 }))
      })
      .catch((e) => setError(errText(e)))
  }, [])

  const finish = async (value: string) => {
    setError('')
    setLoading(true)
    try {
      await api('POST', '/api/auth/mfa/enroll/finish', { code: value })
      onDone()
    } catch (err) {
      setError(errText(err))
      setCode('')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Card title={t('login.enroll_title')} subtitle={t('login.enroll_subtitle', { username: me.username })}>
      <Stack align="center">
        <ErrorAlert error={error} />
        {qr && (
          <Paper withBorder p="sm" bg="white" radius="lg" shadow="sm">
            <Image src={qr} w={190} h={190} alt={t('login.qr_alt')} />
          </Paper>
        )}
        {secret && (
          <Text size="xs" c="dimmed" ta="center">
            {t('login.manual_key')}
            <br />
            <Code>{secret.secret.replace(/(.{4})/g, '$1 ').trim()}</Code>
          </Text>
        )}
        <Text size="sm">{t('login.enter_code')}</Text>
        <PinInput
          length={6}
          type="number"
          oneTimeCode
          value={code}
          onChange={setCode}
          onComplete={finish}
          disabled={loading || !secret}
        />
        <Button
          fullWidth
          size="md"
          variant="gradient"
          loading={loading}
          disabled={code.length !== 6}
          onClick={() => finish(code)}
        >
          {t('login.enable_mfa')}
        </Button>
        <LogoutLink onDone={onDone} />
      </Stack>
    </Card>
  )
}

export function ChangePasswordForm({ onDone, submitLabel }: { onDone: () => void; submitLabel?: string }) {
  const { t } = useTranslation()
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    if (next !== confirm) {
      setError(t('login.passwords_mismatch'))
      return
    }
    setLoading(true)
    try {
      await api('POST', '/api/auth/password', { current_password: current, new_password: next })
      setCurrent('')
      setNext('')
      setConfirm('')
      onDone()
    } catch (err) {
      setError(errText(err))
    } finally {
      setLoading(false)
    }
  }

  return (
    <form onSubmit={submit}>
      <Stack>
        <ErrorAlert error={error} />
        <PasswordInput
          label={t('login.current_password')}
          autoComplete="current-password"
          value={current}
          onChange={(e) => setCurrent(e.currentTarget.value)}
          required
        />
        <PasswordInput
          label={t('login.new_password')}
          description={t('login.new_password_hint')}
          autoComplete="new-password"
          value={next}
          onChange={(e) => setNext(e.currentTarget.value)}
          required
        />
        <PasswordInput
          label={t('login.confirm_password')}
          autoComplete="new-password"
          value={confirm}
          onChange={(e) => setConfirm(e.currentTarget.value)}
          required
        />
        <Button type="submit" loading={loading}>
          {submitLabel ?? t('nav.change_password')}
        </Button>
      </Stack>
    </form>
  )
}

function ChangePasswordStep({ onDone }: { onDone: () => void }) {
  const { t } = useTranslation()
  return (
    <Card title={t('login.change_title')} subtitle={t('login.change_subtitle')}>
      <Stack>
        <ChangePasswordForm onDone={onDone} submitLabel={t('login.save_continue')} />
        <Center>
          <LogoutLink onDone={onDone} />
        </Center>
      </Stack>
    </Card>
  )
}
