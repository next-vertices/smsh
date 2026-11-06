<script lang="ts">
  import Confirm from '../components/Confirm.svelte'
  import { api, ApiError, type Analysis, type CostEstimate, type HistoryItem, type Receivers, type SendOutcome } from '../lib/api'
  import { refreshOverview } from '../lib/actions'
  import { dateFr } from '../lib/format'
  import { ui } from '../lib/state.svelte'
  import { dateTime, money, t } from '../lib/i18n.svelte'

  let recipients = $state('')
  let message = $state('')
  let sender = $state('')
  let scheduled = $state(false)
  let sendAt = $state('')
  let stopClause = $state(false)

  let analysis = $state<Analysis | null>(null)
  let parsed = $state<Receivers | null>(null)
  let confirming = $state(false)
  let sending = $state(false)
  let error = $state('')
  let outcome = $state<SendOutcome | null>(null)

  // Les règles (segments, numéros) vivent côté Go ; on ignore les réponses périmées.
  let seqA = 0, seqR = 0
  $effect(() => {
    const m = message, n = ++seqA
    api.analyze(m).then((a) => { if (n === seqA) analysis = a }).catch(() => {})
  })
  $effect(() => {
    const rc = recipients, n = ++seqR
    api.parseRecipients(rc).then((r) => { if (n === seqR) parsed = r }).catch(() => {})
  })

  // Coût : calculé par le backend (POST /sms/estimate + tarifs par pays) quand la saisie est terminée.
  let cost = $state<CostEstimate | null>(null)
  let costLoading = $state(false)
  let costError = $state('')
  let seqC = 0
  $effect(() => {
    const input = {
      message, recipients, stopClause,
      senderType: ui.overview?.senders.find((x) => x.name === sender)?.type ?? '',
    }
    const n = ++seqC
    cost = null
    costError = ''
    if (!input.message.trim() || !input.recipients.trim()) { costLoading = false; return }
    costLoading = true
    const timer = setTimeout(async () => {
      try {
        const r = await api.estimateCost(input)
        if (n === seqC) cost = r
      } catch (e) {
        if (n === seqC) costError = e instanceof ApiError ? e.message : String(e)
      } finally {
        if (n === seqC) costLoading = false
      }
    }, 600)
    return () => clearTimeout(timer)
  })

  let maxRcpt = $state(5)
  api.maxRecipients().then((n) => (maxRcpt = n)).catch(() => {})

  const eur = money

  function priceText(c: CostEstimate): string {
    if (!(c.priceMax > 0)) return '—'
    if (c.priceMin === c.priceMax) return eur(c.priceMax, c.currency)
    return t('price.range', { min: eur(c.priceMin, c.currency), max: eur(c.priceMax, c.currency) })
  }

  const senders = $derived((ui.overview?.senders ?? []).filter((s) => s.status === 'enable'))
  const validCount = $derived(parsed?.valid.length ?? 0)
  const segments = $derived(cost?.parts || analysis?.segments || 0)
  const estimate = $derived(cost?.totalCredits ?? 0)
  const credits = $derived(ui.overview?.creditsLeft)
  const tooExpensive = $derived(cost !== null && credits !== undefined && estimate > credits)

  const problems = $derived.by(() => {
    const p: string[] = []
    if (!message.trim()) p.push(t('send.need_message'))
    if (!validCount) p.push(t('send.need_recipient'))
    if (validCount > maxRcpt) p.push(t('send.max_exceeded', { max: maxRcpt, got: validCount }))
    if (parsed?.invalid.length) p.push(t('send.invalid_numbers', { numbers: parsed.invalid.join(', ') }))
    if (scheduled && !sendAt) p.push(t('send.need_date'))
    return p
  })
  const canSend = $derived(problems.length === 0 && !sending)

  function ask() {
    if (canSend) { error = ''; confirming = true }
  }

  async function doSend() {
    sending = true
    error = ''
    try {
      outcome = await api.send({
        message, recipients, sender, stopClause,
        sendAt: scheduled ? sendAt : '',
      })
      confirming = false
      message = ''
      recipients = ''
      refreshOverview()
    } catch (e) {
      confirming = false
      lastKind = e instanceof ApiError ? e.kind : ''
      error = e instanceof ApiError ? e.message : String(e)
    } finally {
      sending = false
    }
  }

  function onkeydown(e: KeyboardEvent) {
    if (e.ctrlKey && e.key === 'Enter') { e.preventDefault(); ask() }
  }

  const errorTitle = (kind: string) => {
    const key = `err.${kind}`
    const v = t(key)
    return v === key ? t('err.default') : v
  }
  let lastKind = $state('')

  // Derniers SMS (cache local de l'historique) : panneau affiché sur très grand écran.
  let recent = $state<HistoryItem[]>([])
  api.searchHistory({ query: '', receiver: '', status: '', sender: '', tag: '', dateFrom: '', dateTo: '' }, 0, 40)
    .then((p) => (recent = p.items)).catch(() => {})
</script>

<svelte:window {onkeydown} />

<h1>{t('send.title')}</h1>
<p class="muted">{t('send.intro')} <kbd>Ctrl</kbd>+<kbd>{t('key.enter')}</kbd> {t('send.shortcut')}</p>

<div class="send">
<div class="left">
{#if outcome}
  <div class="alert ok" role="status">
    <strong>{outcome.scheduled ? t('send.sent_scheduled') : t('send.sent')}</strong>
    {t('send.outcome', { recipients: outcome.sent.length, credits: outcome.creditsRemoved, segments: outcome.analysis.segments, encoding: outcome.analysis.encoding === 'gsm7' ? t('send.gsm7') : t('send.unicode') })}
    {#if outcome.ids.length}<div class="small mono">{t('send.ids', { ids: outcome.ids.join(', ') })}</div>{/if}
    {#if outcome.rejected.length}
      <div class="small">{t('send.rejected', { numbers: outcome.rejected.join(', ') })}</div>
    {/if}
  </div>
{/if}

{#if error}
  <div class="alert error" role="alert"><strong>{errorTitle(lastKind)}</strong>{error}</div>
{/if}

<div class="card form">
  <div class="field">
    <label for="rcpt">{t('send.recipients')}</label>
    <textarea id="rcpt" rows="3" bind:value={recipients} placeholder={t('send.rcpt_ph')}></textarea>
    <div class="hint">
      {t('send.rcpt_hint', { max: maxRcpt })}
      {#if parsed && (validCount || parsed.invalid.length)}
        <strong style="color: {validCount > maxRcpt ? 'var(--danger)' : 'var(--text)'}">{t('send.valid_count', { valid: validCount, max: maxRcpt })}</strong>
        {#if parsed.dupes}· {t('send.dupes', { count: parsed.dupes })}{/if}
        {#if parsed.invalid.length}<span style="color: var(--danger)">· {t('send.invalid_count', { count: parsed.invalid.length })}</span>{/if}
      {/if}
    </div>
  </div>

  <div class="field">
    <label for="msg">{t('send.message')}</label>
    <textarea id="msg" rows="5" bind:value={message}></textarea>
    {#if analysis}
      <div class="counter" class:warn={analysis.segments > 1}>
        <span>{t('send.chars', { count: analysis.chars })}</span>
        <span>{t('send.segments_left', { segments: analysis.segments, remaining: analysis.remaining, per: analysis.perSegment })}</span>
        <span class="badge" class:accent={analysis.encoding === 'ucs2'}>
          {analysis.encoding === 'gsm7' ? t('send.gsm7') : t('send.unicode_limit')}
        </span>
      </div>
    {/if}
  </div>

  <div class="grid2">
    <div class="field">
      <label for="sender">{t('send.sender')}</label>
      <select id="sender" bind:value={sender}>
        <option value="">{t('send.sender_default')}</option>
        {#each senders as s}<option value={s.name}>{s.name}</option>{/each}
      </select>
    </div>
    <div class="field">
      <label class="check" for="sched">
        <input id="sched" type="checkbox" bind:checked={scheduled} /> {t('send.scheduled')}
      </label>
      <input type="datetime-local" bind:value={sendAt} disabled={!scheduled} aria-label={t('send.scheduled_aria')} style="margin-top: 6px" />
    </div>
  </div>

  <div class="field">
    <label class="check" for="stop">
      <input id="stop" type="checkbox" bind:checked={stopClause} />
      {t('send.stop')}
    </label>
  </div>
</div>
</div>
<aside class="side">
<div class="card">
  <div class="label2">{t('send.preview')}</div>
  <div class="phone">
    <div class="bubble" class:empty={!message}>{message || t('send.preview_ph')}</div>
    <div class="small muted meta">
      {sender || t('send.sender_default_label')} → {validCount ? t('send.to_recipients', { count: validCount }) : t('send.no_recipient')}
      {#if scheduled && sendAt}· {t('send.on_date', { date: dateTime(sendAt) })}{/if}
    </div>
  </div>

  {#if message.trim() && validCount}
    <div class="cost" class:pending={costLoading} aria-live="polite">
      {#if costLoading}
        <span class="muted">{t('send.cost_loading')}</span>
      {:else if costError}
        <span style="color: var(--danger)">{t('send.cost_unavailable', { error: costError })}</span>
      {:else if cost && cost.lines.length}
        <div class="head">
          <div>
            <div class="label">{t('send.price')}</div>
            <div class="price">{priceText(cost)}</div>
            <div class="small muted">{t('send.price_note')}</div>
          </div>
          <div class="credits">
            <div class="label">{t('send.credits')}</div>
            <div class="big">{cost.totalCredits}</div>
            {#if credits !== undefined}<div class="small muted">{t('send.balance', { from: credits, to: Math.max(credits - cost.totalCredits, 0) })}</div>{/if}
          </div>
        </div>
        <div class="small muted detail">
          {t('send.cost_detail', { parts: cost.parts, encoding: cost.encoding === 'unicode' ? t('send.unicode') : t('send.gsm7'), recipients: cost.recipients })}
        </div>
        {#if cost.lines.length > 1 || cost.lines[0].creditsPerSms !== 1}
          <table class="small">
            <tbody>
              {#each cost.lines as l}
                <tr>
                  <td class="mono">{l.country.toUpperCase()}</td>
                  <td>{t('send.cost_line', { recipients: l.recipients, parts: cost.parts, rate: l.creditsPerSms })}</td>
                  <td class="num">{t('send.cr', { credits: l.credits })}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        {/if}
        {#if cost.unknown.length}
          <div class="small" style="color: var(--warn)">{t('send.unknown_countries', { numbers: cost.unknown.join(', ') })}</div>
        {/if}
        {#if cost.warning}<div class="small" style="color: var(--warn)">{cost.warning}</div>{/if}
      {/if}
    </div>
  {/if}

  {#if tooExpensive}
    <div class="alert warn">{t('send.too_expensive', { estimate, credits })}</div>
  {/if}

  <div class="row between">
    <div class="muted small">{#if problems.length}{problems[0]}{/if}</div>
    <button class="primary" onclick={ask} disabled={!canSend || costLoading}>
      {scheduled ? t('send.schedule') : t('send.send')}…
    </button>
  </div>
</div>
</aside>

<aside class="recent">
  <div class="card">
    <h2>{t('send.recent')}</h2>
    {#each recent as h (h.id)}
      <div class="rc">
        <div class="row between"><span class="mono small">{h.receiver}</span><span class="small muted">{dateFr(h.createdAt)}</span></div>
        <div class="small muted rcmsg">{h.message}</div>
      </div>
    {:else}
      <p class="muted small" style="margin: 0">{t('send.recent_empty')}</p>
    {/each}
  </div>
</aside>
</div>

{#if confirming}
  <Confirm title={t('send.confirm_title')} confirmLabel={scheduled ? t('send.schedule') : t('send.send')} busy={sending}
    onconfirm={doSend} oncancel={() => (confirming = false)}>
    <p>
      {t('send.confirm_pre')} <strong>{t('send.to_recipients', { count: validCount })}</strong> —
      {t('send.chars_segments', { count: segments })}{#if cost}, {t('send.confirm_about')} <strong>{t('app.credits_left', { count: estimate })}</strong>{/if}
      {#if scheduled}, {t('send.on_date', { date: dateTime(sendAt) })}{/if}{t('send.confirm_end')}
    </p>
    <blockquote>{message}</blockquote>
  </Confirm>
{/if}

<style>
  .send { display: block; }
  .recent { display: none; }
  .label2 { font-size: 11px; text-transform: uppercase; letter-spacing: 0.6px; color: var(--muted); margin-bottom: 8px; }
  .phone { margin-bottom: 14px; }
  .bubble {
    background: var(--accent); color: var(--accent-text); border-radius: 16px 16px 16px 4px;
    padding: 10px 14px; white-space: pre-wrap; overflow-wrap: anywhere; max-height: 220px; overflow: auto; width: fit-content; max-width: 100%;
  }
  .bubble.empty { background: var(--surface-2); color: var(--muted); }
  .meta { margin-top: 6px; }
  .rc { padding: 8px 0; border-top: 1px solid var(--border); }
  .rc:first-of-type { border-top: none; }
  .rcmsg { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  /* 17"-21" et au-delà : formulaire à gauche, coût + aperçu à droite (toujours visibles) */
  @media (min-width: 1280px) {
    .send { display: grid; grid-template-columns: minmax(0, 1fr) 420px; gap: 0 20px; align-items: start; }
    .side { position: sticky; top: 0; }
  }
  @media (min-width: 1920px) {
    .send { grid-template-columns: minmax(0, 1fr) 480px; }
  }
  /* 27"-38" : troisième colonne avec les derniers envois */
  @media (min-width: 2400px) {
    .send { grid-template-columns: minmax(480px, 1.15fr) minmax(440px, 1fr) minmax(400px, 1fr); gap: 0 24px; }
    .send { align-items: stretch; height: calc(100vh - 120px); min-height: 520px; }
    .send > :global(*) { min-height: 0; }
    .recent { display: block; }
    .recent .card { height: 100%; overflow-y: auto; margin-bottom: 0; }
    .side { position: static; }
    .send :global(textarea#msg) { min-height: 220px; }
    .send :global(textarea#rcpt) { min-height: 130px; }
  }
  .counter { display: flex; gap: 14px; margin-top: 6px; font-size: 12px; color: var(--muted); align-items: center; flex-wrap: wrap; }
  .counter.warn { color: var(--warn); }
  blockquote {
    margin: 0; padding: 8px 12px; border-left: 3px solid var(--accent);
    background: var(--surface-2); border-radius: 4px;
    white-space: pre-wrap; max-height: 160px; overflow: auto;
  }
</style>
