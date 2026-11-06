<script lang="ts">
  import { untrack } from 'svelte'
  import Confirm from '../components/Confirm.svelte'
  import CsvTemplate from '../components/CsvTemplate.svelte'
  import { api, ApiError, type Campaign, type ImportInfo, type PlanReport } from '../lib/api'
  import { duration, modeLabel, priceRange } from '../lib/format'
  import { ui } from '../lib/state.svelte'
  import { dateTime, t } from '../lib/i18n.svelte'

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
            new Promise<never>((_, rej) => setTimeout(() => rej(new ApiError('unknown', t('wz.sim_timeout'))), 25000)),
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
  <h1>{t('wz.title')}</h1>
  <button onclick={oncancel}>{t('common.back')}</button>
</div>
<p class="muted">{t('wz.intro')}</p>

{#if error}<div class="alert error" role="alert">{error}</div>{/if}

<div class="wz">
<div class="wz-left">
<div class="card">
  <h2>{t('wz.s1')}</h2>
  <div class="row">
    <button onclick={pick}>{imp ? t('wz.change_file') : t('wz.choose_file')}</button>
    {#if imp}<span><strong>{imp.file}</strong> <span class="muted">{t('wz.rows', { count: imp.rows })}</span></span>{/if}
    <span class="grow"></span>
    <CsvTemplate label={t('wz.template_btn')} />
  </div>
  <div class="hint">{t('wz.csv_hint')}</div>
  {#if imp}
    <div class="field" style="margin-top: 12px; max-width: 320px">
      <label for="pcol">{t('wz.phone_col')}</label>
      <select id="pcol" bind:value={phoneCol}>
        {#if phoneCol < 0}<option value={-1}>{t('wz.choose')}</option>{/if}
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
  <h2>{t('wz.s2')}</h2>
  <div class="field">
    <label for="cname">{t('wz.name')}</label>
    <input id="cname" type="text" bind:value={name} placeholder={t('wz.name_ph')} />
  </div>
  <div class="field">
    <label for="ctpl">{t('wz.message')}</label>
    <textarea id="ctpl" rows="4" bind:this={tpl} bind:value={template} disabled={!imp}
      placeholder={t('wz.tpl_ph')}></textarea>
    {#if imp}
      <div class="chips">
        <span class="muted small">{t('wz.insert')}</span>
        {#each imp.headers as h}<button type="button" class="chip" onclick={() => insertVar(h)}>{'{' + h + '}'}</button>{/each}
      </div>
    {/if}
    <div class="hint">{t('send.chars', { count: template.length })}{#if report}{t('wz.up_to', { max: report.maxParts })}{/if}</div>
  </div>

  <div class="field">
    <div class="label2">{t('wz.mode_title')}</div>
    <label class="check" class:off={hasVars}>
      <input type="radio" bind:group={mode} value="batch" disabled={hasVars} />
      <span><strong>{t('mode.batch')}</strong>{t('wz.batch_desc')}</span>
    </label>
    <label class="check" style="margin-top: 6px">
      <input type="radio" bind:group={mode} value="local" />
      <span><strong>{t('mode.local')}</strong>{t('wz.custom_a')} <span class="mono">{t('wz.var_generic')}</span>{t('wz.custom_b')}</span>
    </label>
    {#if hasVars}<div class="hint">{t('wz.vars_required')}</div>{/if}
  </div>

  <div class="grid2">
    <div class="field">
      <label for="csender">{t('send.sender')}</label>
      <select id="csender" bind:value={sender}>
        <option value="">{t('send.sender_default')}</option>
        {#each senders as s}<option value={s.name}>{s.name}</option>{/each}
      </select>
    </div>
    <div class="field">
      <label for="crate">{t('wz.rate')}</label>
      <input id="crate" type="number" min="0.2" max="20" step="0.5" bind:value={rate} disabled={mode !== 'local'} />
      <div class="hint">{t('wz.rate_hint')}</div>
    </div>
  </div>

  <div class="grid2">
    <div class="field">
      <label class="check" for="csched"><input id="csched" type="checkbox" bind:checked={scheduled} /> {t('send.scheduled')}</label>
      <input type="datetime-local" bind:value={sendAt} disabled={!scheduled} aria-label={t('send.scheduled_aria')} style="margin-top: 6px" />
    </div>
    <div class="field">
      <label class="check" for="cstop"><input id="cstop" type="checkbox" bind:checked={stopClause} /> {t('wz.stop')}</label>
    </div>
  </div>
</div>

</div>
<div class="wz-right">
{#if imp && template.trim()}
  <div class="card">
    <h2>{t('wz.s3')} <span class="muted small">{t('wz.dry')}</span></h2>
    {#if simulating}
      <div class="muted">{t('wz.sim_running')}</div>
    {:else if !report}
      <div class="alert warn" style="margin: 0 0 10px">{t('wz.sim_failed')}{error ? t('wz.sim_failed_detail', { error }) : '.'}</div>
      <button onclick={rerun}>{t('wz.rerun')}</button>
    {:else if report}
      <div class="small" style="margin-bottom: 10px">
        <strong>{report.recipients}</strong> {t('wz.valid_recipients')}
        {#if report.invalidCount}· <span style="color: var(--warn)">{t('wz.ignored_rows', { count: report.invalidCount })}</span>{/if}
        {#if report.dupes}· {t('send.dupes', { count: report.dupes })}{/if}
        · {t('wz.mode')} <strong>{modeLabel[report.mode]}</strong>
        {#if report.durationSeconds > 0}· {t('wz.est_duration')} <strong>{duration(report.durationSeconds)}</strong>{/if}
      </div>

      {#each report.blocking as b}<div class="alert error" role="alert">{b}</div>{/each}
      {#each report.warnings as w}<div class="alert warn">{w}</div>{/each}

      {#if report.lines.length}
        <div class="cost">
          <div class="head">
            <div>
              <div class="label">{t('send.price')}</div>
              <div class="price">{priceRange(report.priceMin, report.priceMax, report.currency)}</div>
              <div class="small muted">{t('send.price_note')}</div>
            </div>
            <div class="credits">
              <div class="label">{t('send.credits')}</div>
              <div class="big">{report.totalCredits}</div>
              {#if ui.overview}<div class="small muted">{t('wz.balance', { credits: ui.overview.creditsLeft })}</div>{/if}
            </div>
          </div>
          <div class="small muted detail">{t('wz.total_segments', { count: report.totalParts })}</div>
          {#if report.lines.length > 1 || report.lines[0].creditsPerSms !== 1}
            <table class="small">
              <tbody>
                {#each report.lines as l}
                  <tr><td class="mono">{l.country.toUpperCase()}</td>
                    <td>{t('wz.line', { recipients: l.recipients, rate: l.creditsPerSms })}</td><td class="num">{t('send.cr', { credits: l.credits })}</td></tr>
                {/each}
              </tbody>
            </table>
          {/if}
        </div>
      {/if}

      {#if sample}
        <div class="row between" style="margin-bottom: 6px">
          <span class="label2">{t('wz.preview', { i: previewIdx + 1, n: report.samples.length })} <span class="mono">{sample.phone}</span>
            <span class="badge">{t('wz.segments_badge', { count: sample.parts })}</span></span>
          <span class="row" style="gap: 4px">
            <button disabled={previewIdx === 0} onclick={() => previewIdx--} aria-label={t('wz.prev')}>‹</button>
            <button disabled={previewIdx >= report.samples.length - 1} onclick={() => previewIdx++} aria-label={t('wz.next')}>›</button>
          </span>
        </div>
        <blockquote>{sample.message}</blockquote>
      {/if}

      {#if report.invalid.length}
        <details style="margin-top: 12px">
          <summary class="small muted">{t('wz.ignored_title', { count: report.invalidCount })}</summary>
          <ul class="small">
            {#each report.invalid as i}<li>{t('wz.ignored_line', { line: i.line, value: i.value || t('wz.empty') })}</li>{/each}
            {#if report.invalidCount > report.invalid.length}<li>{t('wz.and_more', { count: report.invalidCount - report.invalid.length })}</li>{/if}
          </ul>
        </details>
      {/if}
    {/if}
  </div>

  <div class="row" style="justify-content: flex-end; margin-bottom: 32px">
    <button onclick={oncancel}>{t('common.cancel')}</button>
    <button class="primary" disabled={!canLaunch} onclick={() => (confirming = true)}>{t('wz.launch')}</button>
  </div>
{:else}
  <div class="card ph muted">{t('wz.placeholder')}</div>
{/if}
</div>
</div>

{#if confirming && report}
  <Confirm title={t('wz.confirm_title')} confirmLabel={t('wz.confirm_label')} busy={launching} onconfirm={launch} oncancel={() => (confirming = false)}>
    <p>
      {t('wz.c_send')} <strong>{t('wz.quoted', { name })}</strong> {t('wz.c_to')} <strong>{t('send.to_recipients', { count: report.recipients })}</strong>
      ({t('send.chars_segments', { count: report.totalParts })}, <strong>{t('app.credits_left', { count: report.totalCredits })}</strong>
      {#if report.priceMax > 0}, {priceRange(report.priceMin, report.priceMax, report.currency)} {t('wz.excl_vat')}{/if})
      {t('wz.c_in_mode')} <strong>{modeLabel[report.mode]}</strong>
      {#if scheduled && sendAt}, {t('send.on_date', { date: dateTime(sendAt) })}{/if}{t('send.confirm_end')}
    </p>
    <p class="muted small">{t('wz.c_note')}</p>
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
