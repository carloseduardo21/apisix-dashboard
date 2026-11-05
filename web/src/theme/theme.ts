import { Badge, Card, Modal, Paper, Table, Tooltip, createTheme } from '@mantine/core'

const font = "'Inter Variable', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif"

export const theme = createTheme({
  primaryColor: 'indigo',
  primaryShade: { light: 6, dark: 5 },
  fontFamily: font,
  fontFamilyMonospace: "ui-monospace, 'SF Mono', 'JetBrains Mono', Menlo, Consolas, monospace",
  headings: { fontFamily: font, fontWeight: '700' },
  defaultRadius: 'md',
  cursorType: 'pointer',
  defaultGradient: { from: 'indigo', to: 'cyan', deg: 135 },
  components: {
    Card: Card.extend({ defaultProps: { withBorder: true, radius: 'md', padding: 'lg' } }),
    Paper: Paper.extend({ defaultProps: { radius: 'md' } }),
    Modal: Modal.extend({ defaultProps: { radius: 'md', overlayProps: { backgroundOpacity: 0.55, blur: 3 } } }),
    // Sentence case reads better than Mantine's default uppercase and never truncates short words.
    Badge: Badge.extend({
      defaultProps: { radius: 'sm' },
      styles: { label: { textTransform: 'none', overflow: 'visible' }, root: { fontWeight: 600 } },
    }),
    Table: Table.extend({
      styles: {
        th: {
          fontSize: 'var(--mantine-font-size-xs)',
          fontWeight: 700,
          textTransform: 'uppercase',
          letterSpacing: '0.03em',
          color: 'var(--mantine-color-dimmed)',
        },
      },
    }),
    Tooltip: Tooltip.extend({ defaultProps: { withArrow: true } }),
  },
})
