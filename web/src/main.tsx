import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MantineProvider } from '@mantine/core'
import { ModalsProvider } from '@mantine/modals'
import { Notifications } from '@mantine/notifications'
import '@fontsource-variable/inter'
import '@mantine/core/styles.css'
import '@mantine/notifications/styles.css'
import '@mantine/spotlight/styles.css'
import './theme/global.css'
import './i18n'
import { theme } from './theme/theme'
import { App } from './App'
import { ApiError } from './api'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // Client errors (4xx) will not fix themselves on retry.
      retry: (count, e) => count < 1 && !(e instanceof ApiError && e.status < 500),
      refetchOnWindowFocus: false,
      staleTime: 10_000,
    },
  },
})

const root = document.getElementById('root')
if (!root) throw new Error('#root element missing from index.html')
createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <MantineProvider theme={theme} defaultColorScheme="auto">
        <ModalsProvider>
          <Notifications position="top-right" />
          <App />
        </ModalsProvider>
      </MantineProvider>
    </QueryClientProvider>
  </StrictMode>,
)
