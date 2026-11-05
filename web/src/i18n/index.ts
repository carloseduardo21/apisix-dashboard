// UI internationalization (i18next). English is the source language and the
// fallback; catalogs live in ./locales and must have the same keys
// (`go test ./internal/i18n`). Placeholders use single braces: "Hello {name}".
import i18n, { changeLanguage } from 'i18next'
import { initReactI18next } from 'react-i18next'
import en from './locales/en.json'
import es from './locales/es.json'
import ptBR from './locales/pt-BR.json'

export const LANGUAGES = [
  { code: 'en', label: 'English' },
  { code: 'pt-BR', label: 'Português (Brasil)' },
  { code: 'es', label: 'Español' },
] as const

export type Language = (typeof LANGUAGES)[number]['code']

const STORAGE_KEY = 'lintel.language'

const isLanguage = (v: unknown): v is Language => LANGUAGES.some((l) => l.code === v)

/** Saved choice, then the browser languages, then English. */
function detectLanguage(): Language {
  try {
    const saved = localStorage.getItem(STORAGE_KEY)
    if (isLanguage(saved)) return saved
  } catch {
    // Storage may be unavailable (private mode, blocked site data).
  }
  for (const tag of navigator.languages ?? [navigator.language]) {
    const lower = tag.toLowerCase()
    if (lower.startsWith('pt')) return 'pt-BR'
    if (lower.startsWith('es')) return 'es'
    if (lower.startsWith('en')) return 'en'
  }
  return 'en'
}

export function setLanguage(lang: Language) {
  try {
    localStorage.setItem(STORAGE_KEY, lang)
  } catch {
    // Not persisted; the choice still applies to this session.
  }
  void changeLanguage(lang)
}

i18n.on('languageChanged', (lng) => {
  document.documentElement.lang = lng
})

// oxlint-disable-next-line import/no-named-as-default-member -- i18next's documented setup API
void i18n.use(initReactI18next).init({
  resources: { en: { translation: en }, 'pt-BR': { translation: ptBR }, es: { translation: es } },
  lng: detectLanguage(),
  fallbackLng: 'en',
  supportedLngs: LANGUAGES.map((l) => l.code),
  interpolation: { escapeValue: false, prefix: '{', suffix: '}' }, // React already escapes
  returnNull: false,
})

export default i18n
