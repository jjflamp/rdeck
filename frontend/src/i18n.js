// i18n — locales are lazy-loaded from JSON; "system" follows navigator.language.
import { createI18n } from 'vue-i18n'
import en from './locales/en.json'
import zhCN from './locales/zh_CN.json'

const messages = {
  en,
  'zh-CN': zhCN,
}

export function detectLocale(setting) {
  if (setting && setting !== 'system') {
    if (setting === 'zh_CN') return 'zh-CN'
    return setting
  }
  const nav = navigator.language || 'en'
  return messages[nav] ? nav : (nav.startsWith('zh') ? 'zh-CN' : 'en')
}

export const i18n = createI18n({
  legacy: false,
  locale: detectLocale('system'),
  fallbackLocale: 'en',
  messages,
})

export function applyLocale(setting) {
  i18n.global.locale.value = detectLocale(setting)
}
