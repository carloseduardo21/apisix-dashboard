import { Select } from '@mantine/core'
import { IconLanguage } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'
import { LANGUAGES, setLanguage, type Language } from '../i18n'

const isLanguage = (v: string | null): v is Language => LANGUAGES.some((l) => l.code === v)

/** Compact language picker (used where the user menu is not available, e.g. login). */
export function LanguageSelect() {
  const { t, i18n } = useTranslation()
  return (
    <Select
      size="xs"
      w={190}
      aria-label={t('common.language')}
      leftSection={<IconLanguage size={14} />}
      data={LANGUAGES.map((l) => ({ value: l.code, label: l.label }))}
      value={i18n.language}
      onChange={(v) => isLanguage(v) && setLanguage(v)}
      allowDeselect={false}
      comboboxProps={{ withinPortal: true }}
    />
  )
}
