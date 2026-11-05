import { createContext, useContext, useEffect, type ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Center, Loader } from '@mantine/core'
import { api, ApiError, setCsrfToken, setUnauthorizedHandler, type Me } from './api'
import { LoginFlow } from './pages/Login'

interface AuthContextValue {
  me: Me
  refresh: () => Promise<unknown>
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth outside AuthGate')
  return ctx
}

async function fetchMe(): Promise<Me | null> {
  try {
    const me = await api<Me>('GET', '/api/auth/me')
    setCsrfToken(me.csrf_token)
    return me
  } catch (e) {
    if (e instanceof ApiError && e.status === 401) return null
    throw e
  }
}

/** Renders the login flow until the user has a complete session. */
export function AuthGate({ children }: { children: ReactNode }) {
  const qc = useQueryClient()
  const { data: me, isPending, refetch } = useQuery({ queryKey: ['me'], queryFn: fetchMe, retry: false })

  useEffect(() => {
    // Any 401 from the API means the session expired: drop cached data.
    setUnauthorizedHandler(() => {
      qc.clear()
      void refetch()
    })
  }, [qc, refetch])

  if (isPending) {
    return (
      <Center h="100vh">
        <Loader />
      </Center>
    )
  }

  if (!me || !me.mfa_ok || me.must_change_password) {
    return <LoginFlow me={me ?? null} onDone={() => refetch()} />
  }

  const logout = async () => {
    try {
      await api('POST', '/api/auth/logout')
    } finally {
      qc.clear()
      await refetch()
    }
  }

  return <AuthContext.Provider value={{ me, refresh: refetch, logout }}>{children}</AuthContext.Provider>
}
