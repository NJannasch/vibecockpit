<script>
  // Memory.svelte — full-text recall across every transcript across every
  // tool. Backed by /api/memory/search (FTS5 BM25 + porter stemming) plus
  // /api/memory/context for the side-drawer that shows ±N messages around
  // a hit. Same backend that the search_memory + get_session_context MCP
  // tools read — what you find here is what an MCP-connected agent finds.

  import { onMount } from "svelte";
  import { providerLabels, providerColors, relativeTime } from "../lib/utils.js";

  let { onlaunch } = $props();

  let query = $state("");
  let providerFilter = $state(""); // comma-separated, "" = all
  let projectFilter = $state("");
  let limit = $state(20);

  let results = $state([]);
  let loading = $state(false);
  let available = $state(true);
  let stats = $state(null);
  let lastQuery = $state("");
  let elapsedMs = $state(null);

  // Drawer state
  let drawer = $state(null); // { result, messages: [], window, loading }

  // Import/export state
  let importStatus = $state(""); // user-visible toast after upload
  let fileInput; // <input type="file"> handle for triggering picker

  // View toggle: 'active' = search results, 'excluded' = tombstones list
  let view = $state("active");
  let tombstones = $state([]);
  let tombstonesLoading = $state(false);

  async function loadStats() {
    try {
      const r = await fetch("/api/memory/stats");
      if (r.ok) stats = await r.json();
    } catch { /* ignore */ }
  }

  async function search() {
    const q = query.trim();
    if (!q) {
      results = [];
      lastQuery = "";
      return;
    }
    loading = true;
    const t0 = performance.now();
    try {
      const params = new URLSearchParams({ q, limit: String(limit) });
      if (providerFilter) params.set("provider", providerFilter);
      if (projectFilter) params.set("project", projectFilter);
      const r = await fetch("/api/memory/search?" + params.toString());
      if (r.ok) {
        const data = await r.json();
        results = data.results || [];
        available = data.available !== false;
        lastQuery = q;
      }
    } catch {
      results = [];
    } finally {
      elapsedMs = Math.round(performance.now() - t0);
      loading = false;
    }
  }

  async function openContext(result) {
    drawer = { result, messages: [], window: 3, loading: true };
    await loadContext(3);
  }

  async function loadContext(window) {
    if (!drawer) return;
    drawer = { ...drawer, window, loading: true };
    try {
      const params = new URLSearchParams({
        session_id: drawer.result.sessionId,
        center: String(drawer.result.messageIdx ?? 0),
        window: String(window),
      });
      const r = await fetch("/api/memory/context?" + params.toString());
      if (r.ok) {
        const data = await r.json();
        drawer = { ...drawer, messages: data.messages || [], loading: false };
      } else {
        drawer = { ...drawer, messages: [], loading: false };
      }
    } catch {
      drawer = { ...drawer, messages: [], loading: false };
    }
  }

  function closeDrawer() {
    drawer = null;
  }

  function launchInTerminal(result) {
    if (onlaunch) onlaunch(result.sessionId, result.provider);
  }

  function exportDb() {
    // Browser does the download via the Content-Disposition header from
    // the server — we just navigate to the URL.
    window.location.href = "/api/memory/export";
  }

  function pickImport() {
    if (fileInput) fileInput.click();
  }

  async function handleImportFile(e) {
    const file = e.target.files?.[0];
    if (!file) return;
    importStatus = `Uploading ${file.name}…`;
    try {
      const fd = new FormData();
      fd.append("file", file);
      const r = await fetch("/api/memory/import", { method: "POST", body: fd });
      if (!r.ok) {
        const err = await r.text();
        importStatus = `Import failed: ${err.slice(0, 200)}`;
      } else {
        const data = await r.json();
        importStatus = `Imported ${data.added} new session${data.added === 1 ? "" : "s"} from ${data.filename}`;
        await loadStats();
        if (lastQuery) await search();
      }
    } catch (err) {
      importStatus = `Import failed: ${err.message}`;
    } finally {
      // Reset the input so picking the same file twice still fires onchange.
      e.target.value = "";
    }
  }

  async function deleteSession(result, ev) {
    ev.stopPropagation(); // don't open the drawer
    const label = result.summary || result.sessionId;
    if (!window.confirm(`Delete this session from memory?\n\n${label}\n\nIt will be tombstoned, so it stays out of search even after re-indexing or future imports. You can restore it from the Excluded list.`)) {
      return;
    }
    const r = await fetch(`/api/memory/session?id=${encodeURIComponent(result.sessionId)}`, { method: "DELETE" });
    if (!r.ok) {
      importStatus = `Delete failed: ${await r.text()}`;
      return;
    }
    results = results.filter((x) => x.sessionId !== result.sessionId);
    await loadStats();
  }

  async function loadTombstones() {
    tombstonesLoading = true;
    try {
      const r = await fetch("/api/memory/tombstones");
      if (r.ok) {
        const data = await r.json();
        tombstones = data.tombstones || [];
      }
    } finally {
      tombstonesLoading = false;
    }
  }

  async function restoreTombstone(t) {
    if (!window.confirm(`Restore "${t.summary || t.sessionId}"?\n\nIt will reappear in search after the next reindex (for local sessions) or the next import (for sessions from another machine).`)) {
      return;
    }
    const r = await fetch(`/api/memory/untombstone?id=${encodeURIComponent(t.sessionId)}`, { method: "POST" });
    if (!r.ok) {
      importStatus = `Restore failed: ${await r.text()}`;
      return;
    }
    tombstones = tombstones.filter((x) => x.sessionId !== t.sessionId);
  }

  function showActive() {
    view = "active";
  }
  async function showExcluded() {
    view = "excluded";
    await loadTombstones();
  }

  function onKeydown(e) {
    if (e.key === "Enter" && !drawer) search();
    if (e.key === "Escape" && drawer) closeDrawer();
  }

  function providerColor(name) {
    return providerColors[name] || "var(--primary)";
  }
  function providerLabel(name) {
    return providerLabels[name] || name;
  }

  onMount(() => {
    loadStats();
    loadTombstones(); // populate the count chip on the Excluded toggle
  });

  let bytesHuman = $derived(() => {
    if (!stats?.bytesOnDisk) return "—";
    const b = stats.bytesOnDisk;
    if (b < 1024) return b + " B";
    if (b < 1024 * 1024) return (b / 1024).toFixed(1) + " KB";
    return (b / 1024 / 1024).toFixed(1) + " MB";
  });
</script>

<svelte:window onkeydown={onKeydown} />

<div class="page-bar">
  <h2 class="page-bar-title">Memory</h2>
  <span class="page-bar-subtitle">Full-text search across every conversation in every tool</span>
  <div class="page-bar-spacer"></div>
  <div class="page-bar-actions">
    {#if stats}
      <span class="memory-stats" title={"Index at " + (stats.path || "")}>
        {stats.sessions ?? 0} sessions · {stats.messages ?? 0} messages · {bytesHuman()}
      </span>
    {/if}
    <div class="memory-view-toggle" role="tablist">
      <button
        class="btn btn-sm"
        class:btn-primary={view === "active"}
        role="tab"
        aria-selected={view === "active"}
        onclick={showActive}
      >Active</button>
      <button
        class="btn btn-sm"
        class:btn-primary={view === "excluded"}
        role="tab"
        aria-selected={view === "excluded"}
        onclick={showExcluded}
      >Excluded{tombstones.length ? ` (${tombstones.length})` : ""}</button>
    </div>
    <button class="btn btn-sm" onclick={exportDb} title="Download a snapshot of the entire index as a single .db file you can move to another machine.">
      Export…
    </button>
    <button class="btn btn-sm" onclick={pickImport} title="Merge another machine's memory.db into this one. Local sessions and tombstones are preserved.">
      Import…
    </button>
    <input type="file" accept=".db,application/octet-stream" bind:this={fileInput} onchange={handleImportFile} style="display:none" />
  </div>
</div>

<main class="memory-page">
  {#if available === false}
    <div class="memory-warning">
      Memory index is not initialized. Run <code>vibecockpit memory reindex</code>
      or check that <code>~/.config/vibecockpit/cache/memory.db</code> is writable.
    </div>
  {/if}

  {#if importStatus}
    <div class="memory-toast">{importStatus}</div>
  {/if}

  {#if view === "active"}
  <div class="memory-search">
    <div class="memory-search-row">
      <input
        type="text"
        class="memory-input"
        placeholder='Recall anything — "auth bug", "jwt validation", "hamburg booking"…'
        bind:value={query}
        autocomplete="off"
      />
      <button class="btn btn-primary" onclick={search} disabled={loading || !query.trim()}>
        {loading ? "Searching…" : "Search"}
      </button>
    </div>
    <div class="memory-search-row memory-filters">
      <input
        type="text"
        class="memory-input memory-input-sm"
        placeholder="Provider (e.g. claude,cursor)"
        bind:value={providerFilter}
        autocomplete="off"
      />
      <input
        type="text"
        class="memory-input memory-input-sm"
        placeholder="Project name contains…"
        bind:value={projectFilter}
        autocomplete="off"
      />
      <select class="memory-input memory-input-sm" bind:value={limit}>
        <option value={10}>10 results</option>
        <option value={20}>20 results</option>
        <option value={50}>50 results</option>
        <option value={100}>100 results</option>
      </select>
    </div>
    <div class="memory-tips">
      Tips: stems automatically (<em>running</em> matches <em>run</em>) ·
      <code>"exact phrase"</code> · <code>auth NOT okta</code> ·
      <code>auth*</code> for prefix · punctuation auto-handled
      (<code>example.com</code>, <code>v1.2.3</code>)
    </div>
  </div>

  {#if lastQuery}
    <div class="memory-summary">
      {#if results.length === 0 && !loading}
        No matches for <strong>{lastQuery}</strong>.
      {:else}
        {results.length} match{results.length === 1 ? "" : "es"} for
        <strong>{lastQuery}</strong>{elapsedMs !== null ? ` (${elapsedMs} ms)` : ""}
      {/if}
    </div>
  {/if}

  <div class="memory-results">
    {#each results as r (r.sessionId)}
      <!-- Outer is a div (not button) so the inner delete button is valid HTML. -->
      <div
        class="memory-result"
        role="button"
        tabindex="0"
        onclick={() => openContext(r)}
        onkeydown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); openContext(r); } }}
        title="Open context"
      >
        <div class="memory-result-head">
          <span class="memory-pill" style="--chip-color:{providerColor(r.provider)}">
            <span class="memory-pill-dot"></span>
            {providerLabel(r.provider)}
          </span>
          <span class="memory-project">{r.projectName || "(no project)"}</span>
          {#if r.gitBranch}<span class="memory-branch">{r.gitBranch}</span>{/if}
          {#if r.host}
            <span class="memory-host" title="Indexed on host {r.host}">
              <svg width="10" height="10" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
                <path d="M2 3h12v8H2V3zm1 1v6h10V4H3zm-1 8h12v1H2v-1z"/>
              </svg>
              {r.host}
            </span>
          {/if}
          <span class="memory-spacer"></span>
          {#if r.messageCount > 0}
            <span class="memory-msgcount">msg {r.messageIdx + 1} / {r.messageCount}</span>
          {/if}
          {#if r.modified}<span class="memory-time">{relativeTime(r.modified)}</span>{/if}
          <button
            type="button"
            class="memory-delete"
            onclick={(e) => deleteSession(r, e)}
            title="Delete (tombstone — restorable from Excluded)"
            aria-label="Delete session"
          >×</button>
        </div>
        {#if r.summary}
          <div class="memory-summary-line">{r.summary}</div>
        {/if}
        {#if r.snippet}
          <!-- snippet contains <mark> tags from the FTS5 backend; safe HTML render. -->
          <div class="memory-snippet">{@html r.snippet}</div>
        {/if}
        <div class="memory-meta">
          {#if r.model}<span>{r.model}</span>{/if}
          <span class="memory-id">{r.sessionId}</span>
        </div>
      </div>
    {/each}
  </div>
  {:else if view === "excluded"}
    <div class="memory-excluded-intro">
      Sessions you've deleted from memory. Tombstoned ids are skipped by
      reindexing and import, so they stay out of search until restored.
    </div>
    {#if tombstonesLoading}
      <div class="memory-summary">Loading…</div>
    {:else if tombstones.length === 0}
      <div class="memory-summary">No excluded sessions.</div>
    {:else}
      <div class="memory-results">
        {#each tombstones as t (t.sessionId)}
          <div class="memory-result memory-result-excluded">
            <div class="memory-result-head">
              {#if t.provider}
                <span class="memory-pill" style="--chip-color:{providerColor(t.provider)}">
                  <span class="memory-pill-dot"></span>
                  {providerLabel(t.provider)}
                </span>
              {/if}
              <span class="memory-project">{t.projectName || "(no project)"}</span>
              {#if t.host}
                <span class="memory-host" title="Originally indexed on {t.host}">
                  <svg width="10" height="10" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
                    <path d="M2 3h12v8H2V3zm1 1v6h10V4H3zm-1 8h12v1H2v-1z"/>
                  </svg>
                  {t.host}
                </span>
              {/if}
              <span class="memory-spacer"></span>
              {#if t.deletedAt}<span class="memory-time">deleted {relativeTime(t.deletedAt)}</span>{/if}
              <button
                type="button"
                class="btn btn-sm"
                onclick={() => restoreTombstone(t)}
                title="Remove the tombstone — session will be re-added on next reindex/import"
              >Restore</button>
            </div>
            {#if t.summary}
              <div class="memory-summary-line">{t.summary}</div>
            {/if}
            <div class="memory-meta">
              <span class="memory-id">{t.sessionId}</span>
            </div>
          </div>
        {/each}
      </div>
    {/if}
  {/if}
</main>

{#if drawer}
  <!-- Real <button> so a click target carries semantic+keyboard a11y for free. -->
  <button type="button" class="memory-drawer-overlay" onclick={closeDrawer} aria-label="Close session context"></button>
  <div class="memory-drawer" role="dialog" aria-modal="true" aria-label="Session context">
    <header class="memory-drawer-head">
      <div class="memory-drawer-title">
        <span class="memory-pill" style="--chip-color:{providerColor(drawer.result.provider)}">
          <span class="memory-pill-dot"></span>
          {providerLabel(drawer.result.provider)}
        </span>
        <strong>{drawer.result.projectName || "(no project)"}</strong>
        {#if drawer.result.host}
          <span class="memory-host" title="Indexed on host {drawer.result.host}">
            <svg width="10" height="10" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
              <path d="M2 3h12v8H2V3zm1 1v6h10V4H3zm-1 8h12v1H2v-1z"/>
            </svg>
            {drawer.result.host}
          </span>
        {/if}
      </div>
      <button class="btn btn-ghost btn-icon" onclick={closeDrawer} aria-label="Close" title="Close (Esc)">×</button>
    </header>

    {#if drawer.result.summary}
      <div class="memory-drawer-summary">{drawer.result.summary}</div>
    {/if}

    <div class="memory-drawer-actions">
      <span class="memory-drawer-meta">
        {#if drawer.result.model}{drawer.result.model} · {/if}
        msg {drawer.result.messageIdx + 1} of {drawer.result.messageCount} ·
        ±{drawer.window}
      </span>
      <span class="memory-drawer-spacer"></span>
      <div class="memory-drawer-window">
        <span>Window:</span>
        {#each [3, 5, 10, 25] as w (w)}
          <button class="btn btn-sm" class:btn-primary={drawer.window === w} onclick={() => loadContext(w)}>±{w}</button>
        {/each}
      </div>
    </div>

    <div class="memory-drawer-body">
      {#if drawer.loading}
        <div class="memory-drawer-empty">Loading context…</div>
      {:else if drawer.messages.length === 0}
        <div class="memory-drawer-empty">No messages around this point.</div>
      {:else}
        {#each drawer.messages as m (m.idx)}
          <div class="memory-msg" class:memory-msg-center={m.isCenter} class:memory-msg-user={m.role === "user"} class:memory-msg-assistant={m.role === "assistant"}>
            <div class="memory-msg-head">
              <span class="memory-msg-role">{m.role}</span>
              <span class="memory-msg-idx">#{m.idx + 1}</span>
              {#if m.timestamp}<span class="memory-msg-time">{relativeTime(m.timestamp)}</span>{/if}
            </div>
            <div class="memory-msg-content">{m.content}</div>
          </div>
        {/each}
      {/if}
    </div>

    <footer class="memory-drawer-foot">
      <button class="btn" onclick={closeDrawer}>Close</button>
      <span class="memory-drawer-spacer"></span>
      <button class="btn btn-primary" onclick={() => launchInTerminal(drawer.result)}>
        Open in terminal
      </button>
    </footer>
  </div>
{/if}
