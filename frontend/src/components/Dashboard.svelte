<script>
  import { onMount } from "svelte";
  import { providerColors, providerLabels, relativeTime, computeDashboardData } from "../lib/utils.js";
  import { fetchBoards } from "../lib/api.js";

  let { sessions, onnavigate, onlaunch, onfilterby } = $props();
  let boards = $state([]);

  onMount(async () => {
    try { boards = await fetchBoards(); } catch { /* optional */ }
  });

  let dashboardData = $derived.by(() => {
    const data = computeDashboardData(sessions);
    data.providers.sort((a, b) => {
      if (b.sessions !== a.sessions) return b.sessions - a.sessions;
      return (b.lastActivity || 0) - (a.lastActivity || 0);
    });
    return data;
  });

  function isRemote(name) {
    return name && name.startsWith("remote-");
  }

  function displayName(name) {
    if (isRemote(name)) return name.replace(/^remote-/, "");
    return providerLabels[name] || name;
  }

  function color(name) {
    const base = isRemote(name) ? name.replace(/^remote-/, "") : name;
    return providerColors[base] || providerColors[name] || "var(--primary)";
  }

  function truncate(text, max) {
    if (!text) return "";
    return text.length > max ? text.slice(0, max - 3) + "..." : text;
  }

  // Split providers into front row (top 2 — one each side of the captain
  // seat) and back row (the rest). Front-row capacity is fixed at 2 by
  // the markup; bumping this past 2 silently drops the extras.
  let frontRow = $derived(dashboardData.providers.slice(0, Math.min(2, dashboardData.providers.length)));
  let backRow = $derived(dashboardData.providers.slice(Math.min(2, dashboardData.providers.length)));
</script>

<div class="cockpit">
  <!-- Windshield -->
  <svg class="windshield" viewBox="0 0 800 60" preserveAspectRatio="none">
    <path d="M0 60 Q0 0 400 0 Q800 0 800 60" fill="none" stroke="var(--border)" stroke-width="1.5"/>
  </svg>

  <!-- Instrument panel: total stats -->
  <div class="instruments">
    <div class="gauge">
      <span class="gauge-value">{dashboardData.totalSessions}</span>
      <span class="gauge-label">sessions</span>
    </div>
    <div class="gauge">
      <span class="gauge-value">{dashboardData.providers.length}</span>
      <span class="gauge-label">sources</span>
    </div>
    <div class="gauge">
      <span class="gauge-value" class:active-glow={dashboardData.totalActive > 0}>{dashboardData.totalActive}</span>
      <span class="gauge-label">active</span>
    </div>
  </div>

  <!-- Cockpit floor -->
  <div class="floor">
    <!-- Back row seats -->
    {#if backRow.length > 0}
      <div class="seat-row back-row">
        {#each backRow as p}
          <button class="seat seat-sm" class:seat-remote={isRemote(p.id)} style="--seat-color:{color(p.id)}" onclick={() => onfilterby(p.id)}>
            <span class="seat-headrest"></span>
            <span class="seat-dot"></span>
            <span class="seat-name">{displayName(p.id)}</span>
            <span class="seat-count">{p.sessions}</span>
            {#if p.active > 0}<span class="seat-active">&#9679;</span>{/if}
          </button>
        {/each}
        <!-- Placeholder seats -->
        <div class="seat seat-placeholder"><span class="seat-plus">+</span></div>
        <div class="seat seat-placeholder"><span class="seat-plus">+</span></div>
      </div>
    {/if}

    <!-- Front row: main copilots + captain -->
    <div class="seat-row front-row">
      {#if frontRow.length > 0}
        <button class="seat seat-lg" style="--seat-color:{color(frontRow[0].id)}" onclick={() => onfilterby(frontRow[0].id)}>
          <span class="seat-headrest"></span>
          <span class="seat-dot"></span>
          <span class="seat-name">{displayName(frontRow[0].id)}</span>
          <span class="seat-count">{frontRow[0].sessions}</span>
          {#if frontRow[0].active > 0}<span class="seat-active">&#9679; {frontRow[0].active}</span>{/if}
        </button>
      {/if}

      <!-- Captain seat (you) -->
      <div class="seat seat-captain">
        <span class="seat-headrest captain-headrest"></span>
        <span class="captain-label">YOU</span>
        <span class="captain-sub">captain</span>
      </div>

      {#if frontRow.length > 1}
        <button class="seat seat-lg" style="--seat-color:{color(frontRow[1].id)}" onclick={() => onfilterby(frontRow[1].id)}>
          <span class="seat-headrest"></span>
          <span class="seat-dot"></span>
          <span class="seat-name">{displayName(frontRow[1].id)}</span>
          <span class="seat-count">{frontRow[1].sessions}</span>
          {#if frontRow[1].active > 0}<span class="seat-active">&#9679; {frontRow[1].active}</span>{/if}
        </button>
      {/if}
    </div>

    <!-- Yoke -->
    <div class="yoke"></div>
  </div>

  <!-- Boards banner -->
  <div class="boards-banner">
    <div class="boards-banner-header">
      <h3 class="recent-heading">Planner</h3>
      <button class="boards-banner-link" onclick={() => onnavigate("planner")}>
        {boards.length > 0 ? "View all" : "Create board"} <span>&rarr;</span>
      </button>
    </div>
    {#if boards.length > 0}
      <div class="boards-banner-list">
        {#each boards as b}
          {@const active = (b.tasks || []).filter(t => t.status !== "archived")}
          {@const working = active.filter(t => t.status === "in-progress").length}
          {@const done = active.filter(t => t.status === "done").length}
          <button class="boards-banner-card" onclick={() => onnavigate("planner")}>
            <span class="boards-banner-name">{b.name}</span>
            <span class="boards-banner-stats">
              {active.length} tasks
              {#if working > 0}<span class="boards-banner-active">&#9679; {working}</span>{/if}
              {#if done > 0}<span class="boards-banner-done">&#10003; {done}</span>{/if}
            </span>
          </button>
        {/each}
      </div>
    {:else}
      <p class="boards-banner-empty">No boards yet — create one to track agentic tasks.</p>
    {/if}
  </div>

  <!-- Recent flights -->
  {#if dashboardData.recentSessions.length > 0}
    <div class="recent">
      <h3 class="recent-heading">Recent flights</h3>
      <div class="recent-list">
        {#each dashboardData.recentSessions as s}
          <button class="recent-row" onclick={() => onlaunch(s.id, s.provider)}>
            <span class="recent-dot" style="background:{color(s.provider)}"></span>
            <span class="recent-project">{s.projectName || "untitled"}</span>
            <span class="recent-summary">{truncate(s.summary || s.firstPrompt || "", 55)}</span>
            {#if s.isActive}<span class="recent-badge">active</span>{/if}
            <span class="recent-time">{relativeTime(s.modified)}</span>
          </button>
        {/each}
      </div>
    </div>
  {/if}

  <div class="cta">
    <button class="btn-view-all" onclick={() => onnavigate("sessions")}>
      View All Sessions <span class="cta-arrow">&rarr;</span>
    </button>
  </div>

  <div class="privacy-notice">
    <span class="privacy-icon">&#128274;</span>
    VibeCockpit scans your local AI tool directories (e.g. <code>~/.claude</code>, <code>~/.codex</code>) to discover sessions, configs, and extensions. All analysis happens entirely on your machine — no data is sent anywhere.
  </div>
</div>

<style>
  .cockpit {
    max-width: 800px;
    margin: 0 auto;
    padding: 0 1.5rem 3rem;
  }

  /* ─── Windshield ─── */
  .windshield {
    width: 100%;
    height: 40px;
    opacity: .4;
    margin-bottom: .5rem;
  }

  /* ─── Instruments ─── */
  .instruments {
    display: flex;
    justify-content: center;
    gap: 2.5rem;
    margin-bottom: 1.5rem;
  }
  .gauge {
    text-align: center;
  }
  .gauge-value {
    display: block;
    font-size: 1.8rem;
    font-weight: 700;
    color: var(--text);
    line-height: 1;
  }
  .gauge-value.active-glow {
    color: var(--success);
  }
  .gauge-label {
    font-size: .72rem;
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 1px;
  }

  /* ─── Floor ─── */
  .floor {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 60px 60px 20px 20px;
    padding: 1.5rem 1.5rem 1rem;
    margin-bottom: 2rem;
    position: relative;
  }

  /* ─── Seat rows ─── */
  .seat-row {
    display: flex;
    justify-content: center;
    gap: .6rem;
    margin-bottom: .8rem;
  }

  .seat {
    display: flex;
    flex-direction: column;
    align-items: center;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 10px;
    cursor: pointer;
    font-family: inherit;
    color: var(--text);
    transition: all 150ms ease;
    position: relative;
    padding: .6rem .5rem .5rem;
  }
  .seat:hover {
    border-color: var(--seat-color, var(--border));
    transform: translateY(-2px);
    box-shadow: 0 4px 12px rgba(0,0,0,.08);
  }

  .seat-sm {
    width: 70px;
    min-height: 60px;
  }
  .seat-lg {
    width: 90px;
    min-height: 72px;
  }

  .seat-headrest {
    position: absolute;
    top: -6px;
    left: 50%;
    transform: translateX(-50%);
    width: 40px;
    height: 8px;
    border-radius: 4px;
    background: var(--seat-color, var(--border));
    opacity: .25;
  }
  .seat-sm .seat-headrest { width: 30px; height: 7px; }

  .seat-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--seat-color, var(--text-secondary));
    margin-bottom: .2rem;
  }
  .seat-name {
    font-size: .65rem;
    font-weight: 500;
    color: var(--text-secondary);
    white-space: nowrap;
  }
  .seat-count {
    font-size: 1rem;
    font-weight: 700;
    line-height: 1.2;
  }
  .seat-sm .seat-count { font-size: .85rem; }
  .seat-active {
    font-size: .55rem;
    color: var(--success);
  }

  .seat-remote {
    border-style: dashed;
  }

  .seat-placeholder {
    background: none;
    border: 1px dashed var(--border);
    cursor: default;
    opacity: .5;
    width: 70px;
    min-height: 60px;
    justify-content: center;
  }
  .seat-placeholder:hover {
    transform: none;
    box-shadow: none;
  }
  .seat-plus {
    font-size: 1.2rem;
    color: var(--border);
  }

  /* ─── Captain ─── */
  .seat-captain {
    width: 100px;
    min-height: 80px;
    background: var(--bg);
    border: 2px solid var(--text-secondary);
    border-radius: 14px;
    cursor: default;
    padding: .8rem .5rem .6rem;
  }
  .seat-captain:hover {
    transform: none;
    box-shadow: none;
  }
  .captain-headrest {
    width: 50px !important;
    height: 10px !important;
    background: var(--text-secondary) !important;
    opacity: .2 !important;
  }
  .captain-label {
    font-size: .9rem;
    font-weight: 700;
    color: var(--text);
    margin-top: .2rem;
  }
  .captain-sub {
    font-size: .6rem;
    color: var(--text-secondary);
  }

  /* ─── Yoke ─── */
  .yoke {
    width: 36px;
    height: 14px;
    border: 1.5px solid var(--border);
    border-radius: 50%;
    margin: 0 auto;
  }

  /* ─── Boards banner ─── */
  .boards-banner { margin-bottom: 1.5rem; }
  .boards-banner-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: .5rem; }
  .boards-banner-link { background: none; border: none; cursor: pointer; font-family: inherit; font-size: .78rem; color: var(--primary); padding: 0; }
  .boards-banner-link:hover { text-decoration: underline; }
  .boards-banner-list { display: flex; gap: .5rem; flex-wrap: wrap; }
  .boards-banner-card { display: flex; align-items: center; justify-content: space-between; gap: .8rem;
    padding: .5rem .8rem; background: var(--surface); border: 1px solid var(--border);
    border-radius: var(--radius-sm); cursor: pointer; font-family: inherit; color: var(--text);
    transition: border-color .15s; flex: 1; min-width: 160px; text-align: left; }
  .boards-banner-card:hover { border-color: var(--primary); }
  .boards-banner-name { font-size: .85rem; font-weight: 600; }
  .boards-banner-stats { font-size: .72rem; color: var(--text-secondary); display: flex; gap: .4rem; align-items: center; }
  .boards-banner-active { color: var(--success); }
  .boards-banner-done { color: var(--text-muted); }
  .boards-banner-empty { font-size: .82rem; color: var(--text-muted); margin: 0; }

  /* ─── Recent ─── */
  .recent {
    margin-bottom: 1.5rem;
  }
  .recent-heading {
    font-size: .9rem;
    font-weight: 600;
    color: var(--text);
    margin-bottom: .5rem;
  }
  .recent-list {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    overflow: hidden;
  }
  .recent-row {
    display: flex;
    align-items: center;
    gap: .6rem;
    width: 100%;
    padding: .55rem .8rem;
    border: none;
    background: none;
    cursor: pointer;
    text-align: left;
    font-family: inherit;
    color: var(--text);
    transition: background 150ms ease;
    border-bottom: 1px solid var(--border);
  }
  .recent-row:last-child { border-bottom: none; }
  .recent-row:hover { background: var(--surface-hover); }
  .recent-dot {
    width: 7px; height: 7px; border-radius: 50%; flex-shrink: 0;
  }
  .recent-project {
    font-weight: 600; font-size: .8rem; white-space: nowrap;
    flex-shrink: 0; max-width: 160px; overflow: hidden; text-overflow: ellipsis;
  }
  .recent-summary {
    flex: 1; font-size: .78rem; color: var(--text-secondary);
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0;
  }
  .recent-badge {
    font-size: .6rem; font-weight: 500; color: var(--success);
    background: var(--success-dim); padding: .1rem .35rem; border-radius: 8px;
    flex-shrink: 0;
  }
  .recent-time {
    font-size: .72rem; color: var(--text-secondary); white-space: nowrap;
    flex-shrink: 0; min-width: 3.5rem; text-align: right;
  }

  /* ─── CTA ─── */
  .cta { text-align: center; }
  .btn-view-all {
    display: inline-flex; align-items: center; gap: .5rem;
    padding: .6rem 1.2rem; border-radius: var(--radius-sm);
    font-size: .85rem; font-weight: 500; cursor: pointer;
    border: 1px solid var(--border); background: var(--surface);
    color: var(--text); font-family: inherit; transition: all 150ms ease;
  }
  .btn-view-all:hover {
    border-color: var(--primary); color: var(--primary); background: var(--primary-glow);
  }
  .cta-arrow { transition: transform 150ms ease; }
  .btn-view-all:hover .cta-arrow { transform: translateX(3px); }

  /* ─── Responsive ─── */
  @media (max-width: 700px) {
    .seat-row { flex-wrap: wrap; }
    .instruments { gap: 1.5rem; }
    .recent-summary { display: none; }
  }
</style>
