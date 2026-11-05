import { BrowserRouter, Navigate, Route, Routes, useParams } from 'react-router'
import { Card } from '@mantine/core'
import { IconKey } from '@tabler/icons-react'
import { PageHeader } from './components/PageHeader'
import { notifications } from '@mantine/notifications'
import { canWrite, isAdmin } from './api'
import { AuthGate, useAuth } from './auth'
import { Layout } from './components/Layout'
import { Audit } from './pages/Audit'
import { Certificates } from './pages/Certificates'
import { KongMigration } from './pages/KongMigration'
import { ChangePasswordForm } from './pages/Login'
import { Overview } from './pages/Overview'
import { ResourcePage } from './pages/ResourcePage'
import { Users } from './pages/Users'
import { RESOURCE_BY_KEY } from './resources'

function ResourceRoute() {
  const { resource = '' } = useParams()
  const def = RESOURCE_BY_KEY[resource]
  if (!def) return <Navigate to="/" replace />
  // key resets page state when switching resources.
  return <ResourcePage key={def.key} def={def} />
}

function AdminOnly({ children }: { children: React.ReactNode }) {
  const { me } = useAuth()
  return isAdmin(me.role) ? children : <Navigate to="/" replace />
}

function WritersOnly({ children }: { children: React.ReactNode }) {
  const { me } = useAuth()
  return canWrite(me.role) ? children : <Navigate to="/" replace />
}

function Account() {
  return (
    <>
      <PageHeader icon={IconKey} title="Alterar senha" description="Ao trocar a senha, suas outras sessões são encerradas." />
      <Card maw={520}>
        <ChangePasswordForm
          onDone={() => notifications.show({ color: 'teal', title: 'Senha alterada', message: 'Suas outras sessões foram encerradas.' })}
        />
      </Card>
    </>
  )
}

export function App() {
  return (
    <AuthGate>
      <BrowserRouter>
        <Routes>
          <Route element={<Layout />}>
            <Route index element={<Overview />} />
            <Route path="r/:resource" element={<ResourceRoute />} />
            <Route path="certificados" element={<Certificates />} />
            <Route path="migracao/kong" element={<WritersOnly><KongMigration /></WritersOnly>} />
            <Route path="conta" element={<Account />} />
            <Route path="usuarios" element={<AdminOnly><Users /></AdminOnly>} />
            <Route path="auditoria" element={<AdminOnly><Audit /></AdminOnly>} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </AuthGate>
  )
}
