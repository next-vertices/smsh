<script lang="ts">
  import Confirm from '../components/Confirm.svelte'
  import { api, ApiError, type Analysis, type Receivers, type SendOutcome } from '../lib/api'
  import { refreshOverview } from '../lib/actions'
  import { ui } from '../lib/state.svelte'

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
    const t = recipients, n = ++seqR
    api.parseRecipients(t).then((r) => { if (n === seqR) parsed = r }).catch(() => {})
  })

  const senders = $derived((ui.overview?.senders ?? []).filter((s) => s.status === 'enable'))
  const validCount = $derived(parsed?.valid.length ?? 0)
  const segments = $derived(analysis?.segments ?? 0)
  const estimate = $derived(segments * validCount)
  const credits = $derived(ui.overview?.creditsLeft)
  const tooExpensive = $derived(credits !== undefined && estimate > credits)

  const problems = $derived.by(() => {
    const p: string[] = []
    if (!message.trim()) p.push('Saisissez un message.')
    if (!validCount) p.push('Ajoutez au moins un destinataire.')
    if (parsed?.invalid.length) p.push(`Numéro(s) invalide(s) : ${parsed.invalid.join(', ')}`)
    if (scheduled && !sendAt) p.push("Choisissez la date d'envoi différé.")
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

  const errorTitle: Record<string, string> = {
    insufficient_credits: 'Crédits insuffisants',
    rate_limit: 'Limite de débit atteinte',
    invalid_receiver: 'Destinataire invalide',
    credentials: 'Identifiants refusés',
    network: 'Réseau indisponible',
  }
  let lastKind = $state('')
</script>

<svelte:window {onkeydown} />

<h1>Envoyer un SMS</h1>
<p class="muted">Envoi unitaire ou vers quelques numéros. <kbd>Ctrl</kbd>+<kbd>Entrée</kbd> pour envoyer.</p>

{#if outcome}
  <div class="alert ok" role="status">
    <strong>{outcome.scheduled ? 'SMS programmé' : 'SMS envoyé'}</strong>
    {outcome.sent.length} destinataire(s) · {outcome.creditsRemoved} crédit(s) débité(s) ·
    {outcome.analysis.segments} segment(s) ({outcome.analysis.encoding === 'gsm7' ? 'GSM-7' : 'Unicode'})
    {#if outcome.ids.length}<div class="small mono">ID : {outcome.ids.join(', ')}</div>{/if}
    {#if outcome.rejected.length}
      <div class="small">Refusés par OVH : {outcome.rejected.join(', ')}</div>
    {/if}
  </div>
{/if}

{#if error}
  <div class="alert error" role="alert"><strong>{errorTitle[lastKind] ?? "Échec de l'envoi"}</strong>{error}</div>
{/if}

<div class="card">
  <div class="field">
    <label for="rcpt">Destinataire(s)</label>
    <textarea id="rcpt" rows="3" bind:value={recipients} placeholder="06 12 34 56 78&#10;+33 6 98 76 54 32"></textarea>
    <div class="hint">
      Un numéro par ligne (ou séparés par des virgules). Format national (06…) ou international (+33…).
      {#if parsed && (validCount || parsed.invalid.length)}
        <strong style="color: var(--text)">{validCount} valide(s)</strong>
        {#if parsed.dupes}· {parsed.dupes} doublon(s) ignoré(s){/if}
        {#if parsed.invalid.length}<span style="color: var(--danger)">· {parsed.invalid.length} invalide(s)</span>{/if}
      {/if}
    </div>
  </div>

  <div class="field">
    <label for="msg">Message</label>
    <textarea id="msg" rows="5" bind:value={message}></textarea>
    {#if analysis}
      <div class="counter" class:warn={analysis.segments > 1}>
        <span>{analysis.chars} caractère(s)</span>
        <span>{analysis.segments} segment(s) · {analysis.remaining} restant(s) sur {analysis.perSegment}</span>
        <span class="badge" class:accent={analysis.encoding === 'ucs2'}>
          {analysis.encoding === 'gsm7' ? 'GSM-7' : 'Unicode (70 car./segment)'}
        </span>
      </div>
    {/if}
  </div>

  <div class="grid2">
    <div class="field">
      <label for="sender">Expéditeur</label>
      <select id="sender" bind:value={sender}>
        <option value="">Par défaut (numéro court)</option>
        {#each senders as s}<option value={s.name}>{s.name}</option>{/each}
      </select>
    </div>
    <div class="field">
      <label class="check" for="sched">
        <input id="sched" type="checkbox" bind:checked={scheduled} /> Envoi différé
      </label>
      <input type="datetime-local" bind:value={sendAt} disabled={!scheduled} aria-label="Date d'envoi différé" style="margin-top: 6px" />
    </div>
  </div>

  <div class="field">
    <label class="check" for="stop">
      <input id="stop" type="checkbox" bind:checked={stopClause} />
      Ajouter la mention « STOP » (SMS commercial)
    </label>
  </div>

  {#if tooExpensive}
    <div class="alert warn">Coût estimé ({estimate}) supérieur à vos crédits ({credits}).</div>
  {/if}

  <div class="row between">
    <div class="muted small">
      {#if validCount && segments}Estimation : {segments} × {validCount} = <strong style="color: var(--text)">{estimate} crédit(s)</strong>
        {#if credits !== undefined}· solde {credits}{/if}
      {:else if problems.length}{problems[0]}{/if}
    </div>
    <button class="primary" onclick={ask} disabled={!canSend}>
      {scheduled ? 'Programmer' : 'Envoyer'}…
    </button>
  </div>
</div>

{#if confirming}
  <Confirm title="Confirmer l'envoi" confirmLabel={scheduled ? 'Programmer' : 'Envoyer'} busy={sending}
    onconfirm={doSend} oncancel={() => (confirming = false)}>
    <p>
      Envoyer ce SMS à <strong>{validCount} destinataire(s)</strong> —
      {segments} segment(s), environ <strong>{estimate} crédit(s)</strong>
      {#if scheduled}, le {new Date(sendAt).toLocaleString('fr-FR')}{/if} ?
    </p>
    <blockquote>{message}</blockquote>
  </Confirm>
{/if}

<style>
  .counter { display: flex; gap: 14px; margin-top: 6px; font-size: 12px; color: var(--muted); align-items: center; flex-wrap: wrap; }
  .counter.warn { color: var(--warn); }
  blockquote {
    margin: 0; padding: 8px 12px; border-left: 3px solid var(--accent);
    background: var(--surface-2); border-radius: 4px;
    white-space: pre-wrap; max-height: 160px; overflow: auto;
  }
</style>
