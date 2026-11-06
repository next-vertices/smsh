// Internationalisation de l'interface : catalogues JSON (src/locales), paramètres nommés {nom},
// pluriels par clés suffixées _one / _other avec le paramètre `count`.
// Langue par défaut : fr-FR ; si le système n'est pas en français : en-GB.
import fr from '../locales/fr-FR.json'
import en from '../locales/en-GB.json'

export const LANGS = ['fr-FR', 'en-GB'] as const
export type Lang = (typeof LANGS)[number]
export type LangPref = 'auto' | Lang

const catalogs: Record<Lang, Record<string, string>> = { 'fr-FR': fr, 'en-GB': en }
const STORAGE_KEY = 'smsh.lang'

/** Tout ce qui commence par « fr » donne fr-FR, le reste en-GB. */
export function normalizeLang(tag: string | undefined | null): Lang {
  return /^fr([-_]|$)/i.test((tag ?? '').trim()) ? 'fr-FR' : 'en-GB'
}

function loadPref(): LangPref {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    if (v === 'fr-FR' || v === 'en-GB') return v
  } catch { /* stockage indisponible */ }
  return 'auto'
}

export const i18n = $state({
  lang: 'fr-FR' as Lang,
  pref: loadPref(),
})

/** Singulier : français 0 et 1, anglais 1. */
function isOne(lang: Lang, n: number): boolean {
  return lang === 'fr-FR' ? n >= 0 && n < 2 : n === 1
}

function lookup(lang: Lang, key: string, params?: Record<string, unknown>): string | undefined {
  const cat = catalogs[lang]
  const n = params?.count
  if (typeof n === 'number') {
    const v = cat[key + (isOne(lang, n) ? '_one' : '_other')]
    if (v !== undefined) return v
  }
  return cat[key]
}

/** Traduit une clé. Lit i18n.lang : les gabarits Svelte se mettent à jour au changement de langue. */
export function t(key: string, params?: Record<string, unknown>): string {
  const lang = i18n.lang
  let msg = lookup(lang, key, params) ?? lookup('fr-FR', key, params)
  if (msg === undefined) return key
  if (params) for (const [k, v] of Object.entries(params)) msg = msg.split(`{${k}}`).join(String(v))
  return msg
}

/** Applique une langue à l'interface (sans toucher à la préférence). */
function apply(lang: Lang) {
  i18n.lang = lang
  document.documentElement.lang = lang === 'fr-FR' ? 'fr' : 'en'
}

/**
 * Au démarrage : préférence enregistrée, sinon langue du système (demandée au cœur Go, repli sur le navigateur).
 * Le cœur est ensuite aligné pour que ses messages (erreurs, avertissements) suivent la même langue.
 */
export async function initLanguage(systemLang: () => Promise<string>, syncCore: (l: Lang) => Promise<unknown>) {
  let lang: Lang
  if (i18n.pref !== 'auto') {
    lang = i18n.pref
  } else {
    try { lang = normalizeLang(await systemLang()) } catch { lang = normalizeLang(navigator.language) }
  }
  apply(lang)
  try { await syncCore(lang) } catch { /* cœur indisponible : messages dans sa langue par défaut */ }
}

/** Change la préférence (auto, fr-FR ou en-GB), l'enregistre et applique la langue. */
export async function setLangPref(pref: LangPref, systemLang: () => Promise<string>, syncCore: (l: Lang) => Promise<unknown>) {
  i18n.pref = pref
  try {
    if (pref === 'auto') localStorage.removeItem(STORAGE_KEY)
    else localStorage.setItem(STORAGE_KEY, pref)
  } catch { /* ignoré */ }
  await initLanguage(systemLang, syncCore)
}

// ---- formats dépendant de la langue ----
export const dateTime = (d: unknown) => (d ? new Date(d as string).toLocaleString(i18n.lang) : '')
export const dateOnly = (d: unknown) => (d ? new Date(d as string).toLocaleDateString(i18n.lang) : '')
export const money = (v: number, cur = 'EUR') =>
  new Intl.NumberFormat(i18n.lang, { style: 'currency', currency: cur || 'EUR', minimumFractionDigits: 2, maximumFractionDigits: 3 }).format(v)
export const num = (v: number) => new Intl.NumberFormat(i18n.lang).format(v)
