import type { ReactNode } from 'react'
import { Group, Stack, Text, ThemeIcon, Title } from '@mantine/core'
import type { Icon } from '@tabler/icons-react'

export function PageHeader({
  title,
  description,
  icon: IconCmp,
  actions,
  badge,
}: {
  title: ReactNode
  description?: ReactNode
  icon?: Icon
  actions?: ReactNode
  badge?: ReactNode
}) {
  return (
    <Group justify="space-between" align="flex-start" wrap="wrap" gap="md" mb="lg">
      <Group gap="md" wrap="nowrap" align="center">
        {IconCmp && (
          <ThemeIcon size={44} radius="md" variant="light">
            <IconCmp size={24} stroke={1.6} />
          </ThemeIcon>
        )}
        <Stack gap={2}>
          <Group gap="xs">
            <Title order={2} fz={26}>
              {title}
            </Title>
            {badge}
          </Group>
          {description && (
            <Text c="dimmed" size="sm">
              {description}
            </Text>
          )}
        </Stack>
      </Group>
      {actions && <Group gap="sm">{actions}</Group>}
    </Group>
  )
}
