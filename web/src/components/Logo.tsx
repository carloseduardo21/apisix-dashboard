import { useId } from 'react'
import { Group, Text } from '@mantine/core'

/** Gateway mark: traffic entering a hexagon and leaving split in two. */
export function LogoMark({ size = 30 }: { size?: number }) {
  const id = useId()
  return (
    <svg width={size} height={size} viewBox="0 0 32 32" aria-hidden="true">
      <defs>
        <linearGradient id={id} x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" style={{ stopColor: 'var(--mantine-color-indigo-6)' }} />
          <stop offset="1" style={{ stopColor: 'var(--mantine-color-cyan-5)' }} />
        </linearGradient>
      </defs>
      <path d="M16 2 28 9v14l-12 7-12-7V9z" fill={`url(#${id})`} />
      <path d="M9 16h6m0 0 5-4.5M15 16l5 4.5" stroke="#fff" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" fill="none" />
      <circle cx="22" cy="11" r="2" fill="#fff" />
      <circle cx="22" cy="21" r="2" fill="#fff" />
    </svg>
  )
}

export function Logo({ size = 30 }: { size?: number }) {
  return (
    <Group gap={10} wrap="nowrap">
      <LogoMark size={size} />
      <div style={{ lineHeight: 1.1 }}>
        <Text fw={800} size="md" lh={1.1}>
          APISIX
        </Text>
        <Text size="xs" c="dimmed" fw={500} lh={1.1}>
          Dashboard
        </Text>
      </div>
    </Group>
  )
}
