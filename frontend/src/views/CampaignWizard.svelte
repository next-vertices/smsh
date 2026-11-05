<script lang="ts">
  import { untrack } from 'svelte'
  import Confirm from '../components/Confirm.svelte'
  import CsvTemplate from '../components/CsvTemplate.svelte'
  import { api, ApiError, type Campaign, type ImportInfo, type PlanReport } from '../lib/api'
  import { duration, modeLabel, priceRange } from '../lib/format'
  import { ui } from '../lib/state.svelte'

  let { oncancel, onlaunched }: { oncancel: () => void; onlaunched: (c: Campaign) => void } = $props()

  let imp = $state<ImportInfo | null>(null)
  let phoneCol = $state(-1)
  let name = $state('')
  let template = $state('')
  let mode = $state<'batch' | 'local'>('batch')
  let rate = $state(3)
  let sender = $state('')
  let stopClause = $state(false)
  let scheduled = $state(false)
  let sendAt = $state('')

  let report = $state<PlanReport | null>(null)
  let simulating = $state(false)
  let error = $state('')
  let previewIdx = $state(0)
  let confirming = $state(false)
  let launching = $state(false)
  let tpl: HTMLTextAreaElement

  const senders = $derived((ui.overview?.senders ?? []).filter((s) => s.status === 'enable'))
  const hasVars = $derived((report?.variables.length ?? 0) > 0)

  // Les variables imposent le mode personnalisé (un batch envoie le même message à tous).
  $effect(() => { if (hasVars && mode === 'batch') mode = 'local' })

  async function pick() {
    error = ''
    try {
      const r = await api.pickCsv()
      if (!r.id) return // annulé
      imp = r
      phoneCol = r.phoneColumn
      report = null
      if (!name) name = r.file.replace(/\.[^.]+$/, '')
    } catch (e) {
      error = e instanceof ApiError ? e.message : String(e)
    }
  }

  let rerunTick = $state(0)
  const rerun = () => { error = ''; rerunTick++ }

  // Simulation automatique (dry-run) 600 ms après la dernière modification.
  // L'effet ne dépend que d'une clé texte : il ne se relance que si une entrée change vraiment.
  const simKey = $derived(JSON.stringify({ rerunTick, importId: imp?.id ?? '', phoneColumn: phoneCol, template, mode, stopClause, senderType: '', rate }))
  let seq = 0
  let simCalls = $state(0)
  let simStart = $state(0)
  $effect(() => {
    const key = simKey
    untrack(() => {
      const n = ++seq
      if (!imp || phoneCol < 0 || !template.trim()) { report = null; simulating = false; return }
      simulating = true
      const input = JSON.parse(key)
      delete input.rerunTick
      timer = setTimeout(async () => {
        simCalls++
        simStart = Date.now()
        try {
          const r = await Promise.race([
            api.simulate(input),
            new Promise<never>((_, rej) => setTimeout(() => rej(new ApiError('unknown', 'La simulation met trop de temps à répondre (25 s).')), 25000)),
          ])
          if (n !== seq) return
          // Tableaux jamais null (Go sérialise une slice nil en null).
          r.invalid ??= []; r.variables ??= []; r.samples ??= []; r.lines ??= []; r.blocking ??= []; r.warnings ??= []
          report = r
          previewIdx = 0
          error = ''
        } catch (e) {
          if (n === seq) { report = null; error = e instanceof ApiError ? e.message : String(e) }
        } finally {
          if (n === seq) simulating = false
        }
      }, 600)
    })
    return () => clearTimeout(timer)
  })
  let timer: ReturnType<typeof setTimeout>

  function insertVar(v: string) {
    const s = tpl.selectionStart ?? template.length
    const e = tpl.selectionEnd ?? s
    template = template.slice(0, s) + `{${v}}` + template.slice(e)
    queueMicrotask(() => { tpl.focus(); tpl.setSelectionRange(s + v.length + 2, s + v.length + 2) })
  }

  const canLaunch = $derived(
    !!report && report.blocking.length === 0 && !simulating && !!name.trim() && !(scheduled && !sendAt) && !launching,
  )

  async function launch() {
    if (!imp) return
    launching = true
    error = ''
    try {
      const c = await api.launch({
        importId: imp.id, phoneColumn: phoneCol, template, mode, stopClause, senderType: '', rate,
        name, sender, sendAt: scheduled ? sendAt : '',
      })
      confirming = false
      onlaunched(c)
    } catch (e) {
      confirming = false
      error = e instanceof ApiError ? e.message : String(e)
    } finally {
      launching = false
    }
  }

  const sample = $derived(report?.samples[previewIdx])
</script>

<div class="row between" style="margin-bottom: 4px">
  <h1>Nouvelle campagne</h1>
  <button onclick={oncancel}>Retour</button>
</div>
<p class="muted">Importez un CSV, rédigez le message, vérifiez la simulation puis lancez.</p>

{#if error}<div class="alert error" role="alert">{error}</div>{/if}

<div class="wz">
<div class="wz-left">
<div class="card">
  <h2>1. Destinataires</h2>
  <div class="row">
    <button onclick={pick}>{imp ? 'Changer de fichier…' : 'Choisir un fichier CSV…'}</button>
    {#if imp}<span><strong>{imp.file}</strong> <span class="muted">· {imp.rows} ligne(s)</span></span>{/if}
    <span class="grow"></span>
    <CsvTemplate label="Modèle CSV" />
  </div>
  <div class="hint">La première ligne doit contenir les en-têtes. Séparateur ; , ou tabulation, UTF-8 ou Excel (Windows-1252).</div>
  {#if imp}
    <div class="field" style="margin-top: 12px; max-width: 320px">
      <label for="pcol">Colonne des numéros</label>
      <select id="pcol" bind:value={phoneCol}>
        {#if phoneCol < 0}<option value={-1}>— choisir —</option>{/if}
        {#each imp.headers as h, i}<option value={i}>{h}</option>{/each}
      </select>
    </div>
    <div class="tablewrap">
      <table class="data">
        <thead><tr>{#each imp.headers as h, i}<th class:sel={i === phoneCol}>{h}</th>{/each}</tr></thead>
        <tbody>
          {#each imp.sample as row}
            <tr>{#each imp.headers as _, i}<td class:sel={i === phoneCol}>{row[i] ?? ''}</td>{/each}</tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

<div class="card" class:disabled={!imp}>
  <h2>2. Message</h2>
  <div class="field">
    <label for="cname">Nom de la campagne</label>
    <input id="cname" type="text" bind:value={name} placeholder="Relance septembre" />
  </div>
  <div class="field">
    <label for="ctpl">Message</label>
    <textarea id="ctpl" rows="4" bind:this={tpl} bind:value={template} disabled={!imp}
      placeholder="Bonjour {'{prenom}'}, votre rendez-vous est confirmé."></textarea>
    {#if imp}
      <div class="chips">
        <span class="muted small">Insérer :</span>
        {#each imp.headers as h}<button type="button" class="chip" onclick={() => insertVar(h)}>{'{' + h + '}'}</button>{/each}
      </div>
    {/if}
    <div class="hint">{template.length} caractère(s){#if report} · jusqu'à {report.maxParts} segment(s) par SMS{/if}</div>
  </div>

  <div class="field">
    <div class="label2">Mode d'envoi</div>
    <label class="check" class:off={hasVars}>
      <input type="radio" bind:group={mode} value="batch" disabled={hasVars} />
      <span><strong>Batch OVH</strong> — même message pour tous ; OVH gère l'envoi (continue application fermée), statistiques de livraison. Pas de pause.</span>
    </label>
    <label class="check" style="margin-top: 6px">
      <input type="radio" bind:group={mode} value="local" />
      <span><strong>Personnalisé</strong> — un SMS par destinataire, variables <span class="mono">{'{colonne}'}</span>, débit limité, pause / reprise / annulation (l'application doit rester ouverte).</span>
    </label>
    {#if hasVars}<div class="hint">Le message contient des variables : le mode Personnalisé est requis.</div>{/if}
  </div>

  <div class="grid2">
    <div class="field">
      <label for="csender">Expéditeur</label>
      <select id="csender" bind:value={sender}>
        <option value="">Par défaut (numéro court)</option>
        {#each senders as s}<option value={s.name}>{s.name}</option>{/each}
      </select>
    </div>
    <div class="field">
      <label for="crate">Débit (SMS / seconde)</label>
      <input id="crate" type="number" min="0.2" max="20" step="0.5" bind:value={rate} disabled={mode !== 'local'} />
      <div class="hint">Mode Personnalisé uniquement.</div>
    </div>
  </div>

  <div class="grid2">
    <div class="field">
      <label class="check" for="csched"><input id="csched" type="checkbox" bind:checked={scheduled} /> Envoi différé</label>
      <input type="datetime-local" bind:value={sendAt} disabled={!scheduled} aria-label="Date d'envoi différé" style="margin-top: 6px" />
    </div>
    <div class="field">
      <label class="check" for="cstop"><input id="cstop" type="checkbox" bind:checked={stopClause} /> Mention « STOP » (SMS commercial)</label>
    </div>
  </div>
</div>

</div>
<div class="wz-right">
{#if imp && template.trim()}
  <div class="card">
    <h2>3. Simulation <span class="muted small">(dry-run : rien n'est envoyé)</span></h2>
    {#if simulating}
      <div class="muted">Simulation en cours… <span class="small">(appel n°{simCalls})</span></div>
    {:else if !report}
      <div class="alert warn" style="margin: 0 0 10px">La simulation n'a pas abouti{error ? ' : ' + error : '.'}</div>
      <button onclick={rerun}>Relancer la simulation</button>
    {:else if report}
      <div class="small" style="margin-bottom: 10px">
        <strong>{report.recipients}</strong> destinataire(s) valide(s)
        {#if report.invalidCount}· <span style="color: var(--warn)">{report.invalidCount} ligne(s) ignorée(s)</span>{/if}
        {#if report.dupes}· {report.dupes} doublon(s) ignoré(s){/if}
        · mode <strong>{modeLabel[report.mode]}</strong>
        {#if report.durationSeconds > 0}· durée estimée <strong>{duration(report.durationSeconds)}</strong>{/if}
      </div>

      {#each report.blocking as b}<div class="alert error" role="alert">{b}</div>{/each}
      {#each report.warnings as w}<div class="alert warn">{w}</div>{/each}

      {#if report.lines.length}
        <div class="cost">
          <div class="head">
            <div>
              <div class="label">Prix estimé</div>
              <div class="price">{priceRange(report.priceMin, report.priceMax, report.currency)}</div>
              <div class="small muted">HT · tarif public OVH selon le pack acheté</div>
            </div>
            <div class="credits">
              <div class="label">Crédits</div>
              <div class="big">{report.totalCredits}</div>
              {#if ui.overview}<div class="small muted">solde : {ui.overview.creditsLeft}</div>{/if}
            </div>
          </div>
          <div class="small muted detail">{report.totalParts} segment(s) au total</div>
          {#if report.lines.length > 1 || report.lines[0].creditsPerSms !== 1}
            <table class="small">
              <tbody>
                {#each report.lines as l}
                  <tr><td class="mono">{l.country.toUpperCase()}</td>
                    <td>{l.recipients} dest. × {l.creditsPerSms} cr./segment</td><td class="num">{l.credits} cr.</td></tr>
                {/each}
              </tbody>
            </table>
          {/if}
        </div>
      {/if}

      {#if sample}
        <div class="row between" style="margin-bottom: 6px">
          <span class="label2">Aperçu {previewIdx + 1} / {report.samples.length} — <span class="mono">{sample.phone}</span>
            <span class="badge">{sample.parts} segment(s)</span></span>
          <span class="row" style="gap: 4px">
            <button disabled={previewIdx === 0} onclick={() => previewIdx--} aria-label="Aperçu précédent">‹</button>
            <button disabled={previewIdx >= report.samples.length - 1} onclick={() => previewIdx++} aria-label="Aperçu suivant">›</button>
          </span>
        </div>
        <blockquote>{sample.message}</blockquote>
      {/if}

      {#if report.invalid.length}
        <details style="margin-top: 12px">
          <summary class="small muted">Lignes ignorées ({report.invalidCount})</summary>
          <ul class="small">
            {#each report.invalid as i}<li>ligne {i.line} : « {i.value || 'vide'} »</li>{/each}
            {#if report.invalidCount > report.invalid.length}<li>… et {report.invalidCount - report.invalid.length} autre(s)</li>{/if}
          </ul>
        </details>
      {/if}
    {/if}
  </div>

  <div class="row" style="justify-content: flex-end; margin-bottom: 32px">
    <button onclick={oncancel}>Annuler</button>
    <button class="primary" disabled={!canLaunch} onclick={() => (confirming = true)}>Lancer la campagne…</button>
  </div>
{:else}
  <div class="card ph muted">La simulation (coût, aperçu, durée) apparaîtra ici une fois le fichier choisi et le message rédigé.</div>
{/if}
</div>
</div>

{#if confirming && report}
  <Confirm title="Lancer la campagne" confirmLabel="Lancer" busy={launching} onconfirm={launch} oncancel={() => (confirming = false)}>
    <p>
      Envoyer <strong>« {name} »</strong> à <strong>{report.recipients} destinataire(s)</strong>
      ({report.totalParts} segment(s), <strong>{report.totalCredits} crédit(s)</strong>
      {#if report.priceMax > 0}, {priceRange(report.priceMin, report.priceMax, report.currency)} HT{/if})
      en mode <strong>{modeLabel[report.mode]}</strong>
      {#if scheduled && sendAt}, le {new Date(sendAt).toLocaleString('fr-FR')}{/if} ?
    </p>
    <p class="muted small">Les SMS déjà envoyés ne peuvent pas être rappelés.</p>
  </Confirm>
{/if}

<style>
  .wz { display: block; }
  .ph { display: none; }
  @media (min-width: 1280px) {
    .wz { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 0 20px; align-items: start; }
    .wz-right { position: sticky; top: 0; }
    .ph { display: block; }
  }
  /* 27"-38" : fichier | message | simulation côte à côte */
  @media (min-width: 2400px) {
    .wz { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) minmax(0, 1fr); }
    .wz-left { display: contents; }
  }
  .disabled { opacity: 0.55; pointer-events: none; }
  .label2 { font-weight: 500; margin-bottom: 4px; }
  .check.off { opacity: 0.45; }
  .check { align-items: flex-start; }
  .check input { margin-top: 3px; flex-shrink: 0; }
  .chips { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; margin-top: 6px; }
  .chip { padding: 1px 9px; font-size: 12px; font-family: Consolas, monospace; border-radius: 999px; }
  .tablewrap { overflow-x: auto; margin-top: 8px; border: 1px solid var(--border); border-radius: 8px; }
  table.data { border-collapse: collapse; width: 100%; min-width: 360px; font-size: 12px; }
  table.data th, table.data td { text-align: left; padding: 4px 10px; white-space: nowrap; border-bottom: 1px solid var(--border); }
  table.data th { background: var(--surface-2); font-weight: 600; }
  table.data tr:last-child td { border-bottom: none; }
  .sel { color: var(--accent); }
  blockquote {
    margin: 0; padding: 8px 12px; border-left: 3px solid var(--accent);
    background: var(--surface-2); border-radius: 4px; white-space: pre-wrap;
  }
</style>
