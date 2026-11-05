import type { ReactNode } from 'react'
import { Stack, Text, ThemeIcon } from '@mantine/core'
import type { Icon } from '@tabler/icons-react'

export function EmptyState({
  icon: IconCmp,
  title,
  description,
  action,
}: {
  icon: Icon
  title: string
  description?: ReactNode
  action?: ReactNode
}) {
  return (
    <Stack align="center" gap="xs" py={48} px="md">
      <ThemeIcon size={56} radius="xl" variant="light" color="gray">
        <IconCmp size={28} stroke={1.5} />
      </ThemeIcon>
      <Text fw={600} mt="xs">
        {title}
      </Text>
      {description && (
        <Text size="sm" c="dimmed" ta="center" maw={420}>
          {description}
        </Text>
      )}
      {action && <div style={{ marginTop: 8 }}>{action}</div>}
    </Stack>
  )
}
