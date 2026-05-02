<script>
  import { onMount } from "svelte";
  import { fetchStats } from "../lib/api.js";
  import { providerColors, relativeTime } from "../lib/utils.js";

  let stats = $state(null);
  let loading = $state(true);
  let error = $state(null);
  let tab = $state("timeline");

  onMount(async () => {
    try {
      stats = await fetchStats();
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  });

  const catColors = {
    tool: "#7c3aed",
    config: "#f97316",
    extension: "#06b6d4",
    memory: "#10b981",
  };

  const catIcons = {
    tool: "T",
    config: "C",
    extension: "E",
    memory: "M",
  };

  function shortDate(d) {
    if (!d) return "—";
    try {
      return new Date(d).toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" });
    } catch { return d; }
  }

  function monthYear(d) {
    if (!d) return "";
    try {
      return new Date(d).toLocaleDateString("en-US", { month: "long", year: "numeric" });
    } catch { return ""; }
  }

  let timelineByMonth = $derived.by(() => {
    if (!stats?.timeline?.length) return [];
    const groups = [];
    let current = null;
    for (const ev of stats.timeline) {
      const my = monthYear(ev.date);
      if (!current || current.month !== my) {
        current = { month: my, events: [] };
        groups.push(current);
      }
      current.events.push(ev);
    }
    groups.reverse();
    for (const g of groups) g.events.reverse();
    return groups;
  });

  let tabs = $derived([
    { id: "timeline", label: "Timeline", count: stats?.timeline?.length ?? 0 },
    { id: "tools", label: "Tools", count: stats?.tools?.length ?? 0 },
    { id: "artifacts", label: "Artifacts", count: stats?.artifacts?.length ?? 0 },
  ]);
</script>

<div class="stats">
  <div class="stats-header">
    <div>
      <h2>Adoption Timeline</h2>
      <p class="stats-subtitle">Your AI tooling journey — a forensic view</p>
    </div>
  </div>

  {#if error}
    <div class="stats-error">{error}</div>
  {:else if loading}
    <div class="stats-loading">Computing adoption stats...</div>
  {:else if stats}

  <!-- Summary strip -->
  <div class="summary-strip">
    <div class="summary-stat hero">
      <span class="stat-value">{stats.summary.daysSinceFirstUse}</span>
      <span class="stat-label">days since first use</span>
    </div>
    <div class="summary-stat">
      <span class="stat-value">{stats.summary.totalSessions}</span>
      <span class="stat-label">total sessions</span>
    </div>
    <div class="summary-stat">
      <span class="stat-value">{stats.tools?.length ?? 0}</span>
      <span class="stat-label">tools used</span>
    </div>
    <div class="summary-stat">
      <span class="stat-value">{stats.summary.totalExtensions}</span>
      <span class="stat-label">AI extensions</span>
    </div>
    <div class="summary-stat">
      <span class="stat-value">{stats.summary.totalInstructions}</span>
      <span class="stat-label">config files</span>
    </div>
    <div class="summary-stat">
      <span class="stat-value">{stats.summary.totalMemories}</span>
      <span class="stat-label">memories</span>
    </div>
  </div>

  <!-- Highlights -->
  <div class="highlights">
    {#if stats.summary.mostActiveTool}
      <div class="highlight-chip">
        <span class="hl-label">Most active tool</span>
        <span class="hl-value" style="color:{providerColors[stats.summary.mostActiveTool] || 'var(--primary)'}">{stats.summary.mostActiveTool}</span>
      </div>
    {/if}
    {#if stats.summary.mostActiveProject}
      <div class="highlight-chip">
        <span class="hl-label">Most active project</span>
        <span class="hl-value">{stats.summary.mostActiveProject}</span>
      </div>
    {/if}
    {#if stats.summary.firstActivityDate}
      <div class="highlight-chip">
        <span class="hl-label">First activity</span>
        <span class="hl-value">{shortDate(stats.summary.firstActivityDate)}</span>
      </div>
    {/if}
    {#if stats.summary.projectsWithConfigs > 0}
      <div class="highlight-chip">
        <span class="hl-label">Projects with AI configs</span>
        <span class="hl-value">{stats.summary.projectsWithConfigs} / {stats.summary.totalProjects}</span>
      </div>
    {/if}
  </div>

  <!-- Tabs -->
  <div class="stats-tabs">
    {#each tabs as t (t.id)}
      <button class="stats-tab" class:active={tab === t.id} onclick={() => tab = t.id}>
        {t.label}
        {#if t.count > 0}<span class="tab-count">{t.count}</span>{/if}
      </button>
    {/each}
  </div>

  <div class="tab-content">

    <!-- TIMELINE -->
    {#if tab === "timeline"}
      {#if !stats.timeline?.length}
        <div class="empty">No timeline events found. Use AI coding tools to build your adoption history.</div>
      {:else}
        <div class="timeline">
          {#each timelineByMonth as group (group.month)}
            <div class="tl-month">{group.month}</div>
            {#each group.events as ev, i (i)}
              <div class="tl-event">
                <div class="tl-line">
                  <span class="tl-dot" style="background:{catColors[ev.category] || 'var(--text-secondary)'}">
                    {catIcons[ev.category] || "?"}
                  </span>
                </div>
                <div class="tl-content">
                  <div class="tl-top">
                    <span class="tl-title">{ev.title}</span>
                    <span class="tl-date">{shortDate(ev.date)}</span>
                  </div>
                  {#if ev.detail}
                    <div class="tl-detail">{ev.detail}</div>
                  {/if}
                  <div class="tl-meta">
                    <span class="tl-cat" style="color:{catColors[ev.category]}">{ev.category}</span>
                    {#if ev.project}
                      <span class="tl-project">{ev.project}</span>
                    {/if}
                    {#if ev.provider}
                      <span class="tl-provider" style="color:{providerColors[ev.provider] || 'var(--text-secondary)'}">{ev.provider}</span>
                    {/if}
                  </div>
                </div>
              </div>
            {/each}
          {/each}
        </div>
      {/if}

    <!-- TOOLS -->
    {:else if tab === "tools"}
      {#if !stats.tools?.length}
        <div class="empty">No tool usage data found.</div>
      {:else}
        <div class="table-wrap">
          <table class="stats-table">
            <thead>
              <tr>
                <th>Tool</th>
                <th>Sessions</th>
                <th>Active Days</th>
                <th>First Session</th>
                <th>Last Session</th>
                <th>Span</th>
              </tr>
            </thead>
            <tbody>
              {#each stats.tools as t (t.provider)}
                {@const spanDays = t.firstSession && t.lastSession ? Math.ceil((new Date(t.lastSession) - new Date(t.firstSession)) / 86400000) : 0}
                <tr>
                  <td>
                    <span class="tool-dot" style="background:{providerColors[t.provider] || 'var(--primary)'}"></span>
                    <strong>{t.name}</strong>
                  </td>
                  <td class="num">{t.totalSessions}</td>
                  <td class="num">{t.activityDays}</td>
                  <td class="muted">{shortDate(t.firstSession)}</td>
                  <td class="muted">{relativeTime(t.lastSession)}</td>
                  <td class="muted">{spanDays > 0 ? spanDays + "d" : "—"}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}

    <!-- ARTIFACTS -->
    {:else if tab === "artifacts"}
      {#if !stats.artifacts?.length}
        <div class="empty">No artifacts found.</div>
      {:else}
        <div class="table-wrap">
          <table class="stats-table">
            <thead>
              <tr>
                <th></th>
                <th>Name</th>
                <th>Type</th>
                <th>Project / IDE</th>
                <th>Date</th>
                <th>Age</th>
              </tr>
            </thead>
            <tbody>
              {#each stats.artifacts as a, i (i)}
                <tr>
                  <td>
                    <span class="art-dot" style="background:{catColors[a.category] || 'var(--text-secondary)'}">
                      {catIcons[a.category] || "?"}
                    </span>
                  </td>
                  <td><strong>{a.name}</strong></td>
                  <td class="muted">{a.type}</td>
                  <td class="muted">{a.project || a.source || "—"}</td>
                  <td class="muted">{shortDate(a.date)}</td>
                  <td class="muted">{a.date ? relativeTime(a.date) : "—"}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    {/if}
  </div>

  {/if}
</div>

<style>
  .stats { max-width: 920px; margin: 1.5rem auto; padding: 0 1rem; }
  .stats-header { display: flex; align-items: flex-start; justify-content: space-between; margin-bottom: 1.2rem; }
  .stats-header h2 { margin: 0 0 .2rem; }
  .stats-subtitle { font-size: .82rem; color: var(--text-secondary); margin: 0; }
  .stats-error { color: var(--danger, #ef4444); padding: 1rem; background: var(--surface); border-radius: 8px; }
  .stats-loading { text-align: center; padding: 3rem; color: var(--text-secondary); }

  /* Summary strip */
  .summary-strip {
    display: flex; gap: .5rem; margin-bottom: 1rem; flex-wrap: wrap;
  }
  .summary-stat {
    flex: 1; min-width: 90px;
    background: var(--surface); border: 1px solid var(--border); border-radius: 8px;
    padding: .6rem .8rem; text-align: center;
  }
  .summary-stat.hero {
    border-color: var(--primary);
    background: color-mix(in srgb, var(--primary) 6%, var(--surface));
  }
  .stat-value { display: block; font-size: 1.3rem; font-weight: 700; font-variant-numeric: tabular-nums; color: var(--primary); }
  .stat-label { font-size: .7rem; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .03em; }

  /* Highlights */
  .highlights {
    display: flex; gap: .5rem; margin-bottom: 1.2rem; flex-wrap: wrap;
  }
  .highlight-chip {
    display: flex; flex-direction: column; gap: .1rem;
    background: var(--surface); border: 1px solid var(--border); border-radius: 8px;
    padding: .45rem .75rem; min-width: 130px;
  }
  .hl-label { font-size: .68rem; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .03em; }
  .hl-value { font-size: .88rem; font-weight: 600; }

  /* Tabs */
  .stats-tabs {
    display: flex; gap: .15rem; border-bottom: 1px solid var(--border);
    margin-bottom: 1rem; overflow-x: auto;
  }
  .stats-tab {
    padding: .5rem .9rem; border: none; background: none; cursor: pointer;
    font-size: .82rem; font-weight: 500; color: var(--text-secondary);
    border-bottom: 2px solid transparent; transition: all .15s;
    font-family: inherit; white-space: nowrap;
    display: flex; align-items: center; gap: .35rem;
  }
  .stats-tab:hover { color: var(--text-primary, var(--text)); }
  .stats-tab.active { color: var(--primary); border-bottom-color: var(--primary); font-weight: 600; }
  .tab-count {
    font-size: .68rem; background: var(--surface); border-radius: 8px;
    padding: .05rem .4rem; font-variant-numeric: tabular-nums;
    color: var(--text-secondary);
  }
  .stats-tab.active .tab-count { background: color-mix(in srgb, var(--primary) 12%, transparent); color: var(--primary); }
  .tab-content { min-height: 200px; }

  /* Timeline */
  .timeline { padding-left: .5rem; }
  .tl-month {
    font-size: .78rem; font-weight: 700; color: var(--text-secondary);
    text-transform: uppercase; letter-spacing: .04em;
    padding: .6rem 0 .3rem 2.2rem;
    border-left: 2px solid var(--border);
    margin-left: .55rem;
  }
  .tl-month:first-child { padding-top: 0; }
  .tl-event {
    display: flex; gap: 0; min-height: 2.4rem;
  }
  .tl-line {
    width: 1.7rem; flex-shrink: 0; display: flex; flex-direction: column; align-items: center;
    position: relative;
  }
  .tl-line::before {
    content: ""; position: absolute; top: 0; bottom: 0; left: 50%; width: 2px;
    background: var(--border); transform: translateX(-50%);
  }
  .tl-dot {
    position: relative; z-index: 1;
    width: 1.2rem; height: 1.2rem; border-radius: 50%;
    display: flex; align-items: center; justify-content: center;
    font-size: .52rem; font-weight: 700; color: white;
    margin-top: .35rem; flex-shrink: 0;
  }
  .tl-content {
    flex: 1; padding: .3rem 0 .6rem .5rem; min-width: 0;
  }
  .tl-top { display: flex; align-items: baseline; gap: .5rem; }
  .tl-title { font-size: .84rem; font-weight: 600; }
  .tl-date { font-size: .72rem; color: var(--text-secondary); margin-left: auto; white-space: nowrap; font-variant-numeric: tabular-nums; }
  .tl-detail {
    font-size: .74rem; color: var(--text-secondary); margin-top: .1rem;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .tl-meta { display: flex; gap: .5rem; margin-top: .15rem; font-size: .68rem; }
  .tl-cat { font-weight: 500; text-transform: uppercase; letter-spacing: .03em; }
  .tl-project { color: var(--text-secondary); }
  .tl-provider { font-weight: 500; }

  /* Tables */
  .table-wrap { overflow-x: auto; }
  .stats-table { width: 100%; border-collapse: collapse; font-size: .82rem; }
  .stats-table th { text-align: left; color: var(--text-secondary); font-weight: 500; padding: .4rem .6rem; border-bottom: 1px solid var(--border); }
  .stats-table td { padding: .45rem .6rem; border-bottom: 1px solid var(--border-light, var(--border)); vertical-align: middle; }
  .num { text-align: right; font-variant-numeric: tabular-nums; }
  .muted { color: var(--text-secondary); font-size: .78rem; }
  .tool-dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; margin-right: .35rem; vertical-align: middle; }
  .art-dot {
    display: inline-flex; align-items: center; justify-content: center;
    width: 1.1rem; height: 1.1rem; border-radius: 50%;
    font-size: .48rem; font-weight: 700; color: white;
  }
  .empty { text-align: center; padding: 2.5rem 1rem; color: var(--text-secondary); font-size: .85rem; }
</style>
