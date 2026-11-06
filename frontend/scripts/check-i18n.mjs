// Vérifie la cohérence des traductions de l'interface :
//  - mêmes clés (variantes de pluriel comprises) et mêmes paramètres {nom} dans toutes les langues ;
//  - chaque clé utilisée dans le code (t('...')) existe dans toutes les langues ;
//  - signale les clés inutilisées (avertissement).
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = join(dirname(fileURLToPath(import.meta.url)), '..', 'src')
const LANGS = ['fr-FR', 'en-GB']
// Clés dont les accolades sont du texte littéral (exemples de variables CSV), propre à chaque langue.
const LITERAL_BRACES = new Set(['wz.var_generic', 'wz.tpl_ph'])

const cat = Object.fromEntries(LANGS.map((l) => [l, JSON.parse(readFileSync(join(root, 'locales', `${l}.json`), 'utf8'))]))
const base = (k) => k.replace(/_(one|other)$/, '')
const params = (s) => [...s.matchAll(/\{([a-z_]+)\}/g)].map((m) => m[1]).sort().join(',')

let errors = 0
const fail = (m) => { errors++; console.error('✗ ' + m) }

// 1. parité des clés et des paramètres
const bases = new Map() // clé de base -> paramètres de référence
for (const l of LANGS) {
  for (const [k, v] of Object.entries(cat[l])) {
    const b = base(k)
    if (LITERAL_BRACES.has(b)) continue
    const p = params(v)
    if (bases.has(b) && bases.get(b) !== p) fail(`${k} (${l}) : paramètres {${p}} ≠ {${bases.get(b)}}`)
    bases.set(b, p)
  }
}
for (const l of LANGS) {
  const have = new Set(Object.keys(cat[l]).map(base))
  for (const b of new Set([...LANGS.flatMap((x) => Object.keys(cat[x]).map(base))])) {
    if (!have.has(b)) fail(`clé « ${b} » absente de ${l}`)
  }
}

// 2. clés utilisées dans le code
function walk(dir) {
  return readdirSync(dir).flatMap((f) => {
    const p = join(dir, f)
    return statSync(p).isDirectory() ? walk(p) : /\.(svelte|ts)$/.test(f) ? [p] : []
  })
}
const used = new Set()
for (const file of walk(root)) {
  const src = readFileSync(file, 'utf8')
  for (const m of src.matchAll(/\bt\(\s*(['"`])([a-z0-9_.]+)\1/g)) {
    used.add(m[2])
    for (const l of LANGS) {
      const k = m[2]
      if (!(k in cat[l]) && !(k + '_one' in cat[l]) && !(k + '_other' in cat[l])) fail(`${file.replace(root, 'src')} : clé « ${k} » absente de ${l}`)
    }
  }
}
// clés construites dynamiquement (t(`prefix.${x}`)) : on ne les signale pas comme inutilisées
const dynamic = ['status.', 'mode.', 'err.', 'rec.', 'hist.st_', 'settings.backend_', 'nav.']
const unused = [...bases.keys()].filter((b) => !used.has(b) && !dynamic.some((d) => b.startsWith(d)))
if (unused.length) console.warn('⚠ clés peut-être inutilisées : ' + unused.join(', '))

const n = bases.size
if (errors) { console.error(`\n${errors} erreur(s) i18n`); process.exit(1) }
console.log(`i18n OK : ${n} clés, ${LANGS.length} langues`)
