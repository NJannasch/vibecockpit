<script>
  import { onMount } from "svelte";
  import { providerColors, providerLabels, relativeTime, computeDashboardData } from "../lib/utils.js";
  import { fetchBoards } from "../lib/api.js";

  let { sessions, onnavigate, onlaunch, onfilterby, mcpEnabled = false } = $props();
  let mcpHintDismissed = $state(localStorage.getItem("vc-mcp-hint-dismissed") === "1");

  function dismissMcpHint() {
    localStorage.setItem("vc-mcp-hint-dismissed", "1");
    mcpHintDismissed = true;
  }
  let boards = $state([]);

  let dashboardData = $derived.by(() => {
    const data = computeDashboardData(sessions);
    data.providers.sort((a, b) => {
      if (b.sessions !== a.sessions) return b.sessions - a.sessions;
      return (b.lastActivity || 0) - (a.lastActivity || 0);
    });
    return data;
  });

  function displayName(name) {
    if (name && name.startsWith("remote-")) return name.replace(/^remote-/, "");
    return providerLabels[name] || name;
  }

  function color(name) {
    const base = name && name.startsWith("remote-") ? name.replace(/^remote-/, "") : name;
    return providerColors[base] || providerColors[name] || "var(--primary)";
  }

  function truncate(text, max) {
    if (!text) return "";
    return text.length > max ? text.slice(0, max - 3) + "..." : text;
  }

  let activeSessions = $derived(sessions.filter(s => s.isActive));
  let totalEstCost = $derived(sessions.reduce((sum, s) => sum + (s.estCostUsd || 0), 0));
  let inProgressTasks = $derived(boards.flatMap(b => (b.tasks || []).filter(t => t.status === "in-progress")));

  let agentRuns = $state([]);
  let agentLog = $state({ open: false, taskId: "", stdout: "", status: "", recent: "" });

  async function loadAgents() {
    try {
      const r = await fetch("/api/agents");
      if (r.ok) agentRuns = await r.json();
    } catch { /* ignore */ }
  }

  async function stopAgent(taskId) {
    try {
      await fetch(`/api/agents/${taskId}/stop`, { method: "POST" });
      setTimeout(loadAgents, 1000);
    } catch { /* ignore */ }
  }

  async function viewLog(taskId) {
    try {
      const r = await fetch(`/api/agents/${encodeURIComponent(taskId)}/log`);
      if (r.ok) {
        const d = await r.json();
        agentLog = { open: true, taskId, ...d };
      }
    } catch { /* ignore */ }
  }

  let agentPollTimer;
  onMount(async () => {
    try { boards = await fetchBoards(); } catch { /* optional */ }
    loadAgents();
    agentPollTimer = setInterval(loadAgents, 5000);
  });

  import { onDestroy } from "svelte";
  onDestroy(() => { clearInterval(agentPollTimer); });
</script>

<div class="dash">
  <!-- Metric cards -->
  <div class="metrics">
    <div class="metric-card">
      <span class="metric-value">{sessions.length}</span>
      <span class="metric-label">sessions</span>
    </div>
    <div class="metric-card">
      <span class="metric-value" class:metric-active={activeSessions.length > 0}>{activeSessions.length}</span>
      <span class="metric-label">active</span>
    </div>
    <div class="metric-card">
      <span class="metric-value">{dashboardData.providers.length}</span>
      <span class="metric-label">tools</span>
    </div>
    <div class="metric-card">
      <span class="metric-value">~${totalEstCost >= 1000 ? (totalEstCost/1000).toFixed(1) + "k" : totalEstCost.toFixed(0)}</span>
      <span class="metric-label">est. cost</span>
    </div>
  </div>

  <!-- Running agents -->
  {#if agentRuns.length > 0}
    {@const runningCount = agentRuns.filter(r => r.status === "running").length}
    {@const recentRuns = [...agentRuns].sort((a, b) => new Date(b.startedAt) - new Date(a.startedAt)).slice(0, 4)}
    <div class="agents-panel" class:agents-active={runningCount > 0}>
      <div class="agents-header">
        <h3 class="agents-title">Agents ({runningCount} running)</h3>
        <button class="agents-viewall" onclick={() => onnavigate("agents")}>View all ({agentRuns.length})</button>
      </div>
      {#each recentRuns as run (run.taskId)}
        <div class="agent-row">
          {#if run.status === "running"}
            <span class="agent-pulse"></span>
          {:else if run.status === "completed"}
            <span class="agent-dot agent-done">&#10003;</span>
          {:else}
            <span class="agent-dot agent-fail">&#10005;</span>
          {/if}
          <div class="agent-info">
            <span class="agent-task">{run.taskTitle || run.taskId}</span>
            <span class="agent-meta">{run.tool}{run.model ? " · " + run.model : ""} · {run.status === "running" ? run.elapsed : run.status}</span>
          </div>
          {#if run.logPath}
            <button class="btn btn-sm" onclick={() => viewLog(run.taskId)} title="View output">Log</button>
          {/if}
          {#if run.status === "running"}
            <button class="btn btn-sm btn-danger" onclick={() => stopAgent(run.taskId)} title="Stop agent">Stop</button>
          {/if}
        </div>
      {/each}
    </div>
  {/if}

  {#if agentLog.open}
    <div class="agent-log-panel">
      <div class="agent-log-header">
        <span>Agent: {agentLog.taskId}</span>
        <div>
          <button class="btn btn-sm" onclick={() => viewLog(agentLog.taskId)}>Refresh</button>
          <button class="btn btn-sm" onclick={() => { agentLog = { open: false, taskId: "", stdout: "", status: "", recent: "" }; }}>Close</button>
        </div>
      </div>
      {#if agentLog.status}
        <div class="agent-log-section">STATUS.md</div>
        <pre class="agent-log-content">{agentLog.status}</pre>
      {/if}
      {#if agentLog.recent}
        <div class="agent-log-section">Recent activity</div>
        <pre class="agent-log-content">{agentLog.recent}</pre>
      {/if}
      <div class="agent-log-section">Stdout</div>
      <pre class="agent-log-content">{agentLog.stdout || "(empty)"}</pre>
    </div>
  {/if}

  <!-- Two-column layout -->
  <div class="dash-grid">
    <!-- Left: active sessions + providers -->
    <div class="dash-col">
      {#if activeSessions.length > 0}
        <div class="dash-card">
          <h3 class="dash-card-title">Active sessions</h3>
          {#each activeSessions as s (s.id)}
            <button class="active-row" onclick={() => onlaunch(s.id, s.provider)}>
              <span class="active-dot" style="background:{color(s.provider)}"></span>
              <span class="active-project">{s.projectName || "untitled"}</span>
              <span class="active-meta">{displayName(s.provider)}</span>
              {#if s.estCostUsd > 0}<span class="active-cost">~${s.estCostUsd.toFixed(0)}</span>{/if}
            </button>
          {/each}
        </div>
      {/if}

      <div class="dash-card">
        <h3 class="dash-card-title">Tools</h3>
        <div class="tool-grid">
          {#each dashboardData.providers as p (p.id)}
            <button class="tool-chip" style="--chip-color:{color(p.id)}" onclick={() => onfilterby(p.id)}>
              <span class="tool-dot"></span>
              <span class="tool-name">{displayName(p.id)}</span>
              <span class="tool-count">{p.sessions}</span>
              {#if p.active > 0}<span class="tool-active">&#9679;</span>{/if}
            </button>
          {/each}
        </div>
      </div>
    </div>

    <!-- Right: boards + recent -->
    <div class="dash-col">
      <div class="dash-card">
        <div class="dash-card-header">
          <h3 class="dash-card-title">Planner</h3>
          <button class="dash-link" onclick={() => onnavigate("planner")}>{boards.length > 0 ? "Open" : "Create"} &rarr;</button>
        </div>
        {#if boards.length > 0}
          {#each boards as b (b.name)}
            {@const active = (b.tasks || []).filter(t => t.status !== "archived")}
            {@const working = active.filter(t => t.status === "in-progress").length}
            <button class="board-row" onclick={() => onnavigate("planner")}>
              <span class="board-row-name">{b.name}</span>
              <span class="board-row-stats">
                {active.length} tasks
                {#if working > 0}<span class="board-row-active">&#9679; {working}</span>{/if}
              </span>
            </button>
          {/each}
          {#if inProgressTasks.length > 0}
            <div class="in-progress-list">
              {#each inProgressTasks as t (t.id)}
                <div class="in-progress-item">
                  <span class="in-progress-dot"></span>
                  <span class="in-progress-title">{truncate(t.title, 40)}</span>
                </div>
              {/each}
            </div>
          {/if}
        {:else}
          <p class="dash-empty">No boards yet — track agentic tasks with cost per feature.</p>
        {/if}
      </div>

      {#if dashboardData.recentSessions.length > 0}
        <div class="dash-card">
          <div class="dash-card-header">
            <h3 class="dash-card-title">Recent sessions</h3>
            <button class="dash-link" onclick={() => onnavigate("sessions")}>All &rarr;</button>
          </div>
          {#each dashboardData.recentSessions.slice(0, 6) as s (s.id)}
            <button class="recent-row" onclick={() => onlaunch(s.id, s.provider)}>
              <span class="recent-dot" style="background:{color(s.provider)}"></span>
              <span class="recent-project">{s.projectName || "untitled"}</span>
              <span class="recent-summary">{truncate(s.summary || s.firstPrompt || "", 40)}</span>
              {#if s.isActive}<span class="recent-badge">active</span>{/if}
              <span class="recent-time">{relativeTime(s.modified)}</span>
            </button>
          {/each}
        </div>
      {/if}
    </div>
  </div>

  {#if !mcpEnabled && !mcpHintDismissed && sessions.length > 0}
    <div class="mcp-hint">
      <div class="mcp-hint-content">
        <strong>Connect your agents</strong>
        <span>Enable MCP in Settings and add <code>.mcp.json</code> to your project — your AI agents can then track tasks, link sessions, and report costs automatically.</span>
      </div>
      <div class="mcp-hint-actions">
        <button class="btn btn-sm btn-primary" onclick={() => onnavigate("settings")}>Enable MCP</button>
        <button class="btn btn-sm" onclick={dismissMcpHint}>Dismiss</button>
      </div>
    </div>
  {/if}
</div>

<style>
  .dash { max-width: 900px; margin: 0 auto; padding: 0 1.5rem 2rem; }

  /* Metrics */
  .metrics { display: flex; gap: .6rem; margin-bottom: 1rem; }
  .metric-card { flex: 1; text-align: center; padding: .7rem .5rem; background: var(--surface);
    border: 1px solid var(--border); border-radius: var(--radius-sm); }
  .metric-value { display: block; font-size: 1.4rem; font-weight: 700; color: var(--text); line-height: 1.2; }
  .metric-value.metric-active { color: var(--success); }
  .metric-label { font-size: .68rem; color: var(--text-muted); text-transform: uppercase; letter-spacing: .5px; }

  /* Grid */
  .dash-grid { display: grid; grid-template-columns: 1fr 1fr; gap: .8rem; }

  /* Cards */
  .dash-card { background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius); padding: .8rem; margin-bottom: .8rem; }
  .dash-card:last-child { margin-bottom: 0; }
  .dash-card-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: .4rem; }
  .dash-card-title { font-size: .82rem; font-weight: 600; color: var(--text); margin: 0 0 .4rem; }
  .dash-card-header .dash-card-title { margin: 0; }
  .dash-link { background: none; border: none; font-family: inherit; font-size: .75rem; color: var(--primary); cursor: pointer; padding: 0; }
  .dash-link:hover { text-decoration: underline; }
  .dash-empty { font-size: .8rem; color: var(--text-muted); margin: 0; }

  /* Active sessions */
  .active-row { display: flex; align-items: center; gap: .5rem; width: 100%; padding: .35rem .5rem;
    background: none; border: 1px solid var(--border); border-radius: var(--radius-sm);
    cursor: pointer; font-family: inherit; color: var(--text); text-align: left; margin-bottom: .3rem; transition: border-color .15s; }
  .active-row:hover { border-color: var(--primary); }
  .active-dot { width: 7px; height: 7px; border-radius: 50%; flex-shrink: 0; animation: pulse 2s infinite; }
  @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: .4; } }
  .active-project { font-size: .82rem; font-weight: 600; flex: 1; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .active-meta { font-size: .7rem; color: var(--text-muted); flex-shrink: 0; }
  .active-cost { font-size: .72rem; font-weight: 600; color: var(--warning, #f59e0b); flex-shrink: 0; }

  /* Tools */
  .tool-grid { display: flex; flex-wrap: wrap; gap: .3rem; }
  .tool-chip { display: flex; align-items: center; gap: .3rem; padding: .25rem .5rem;
    background: var(--bg); border: 1px solid var(--border); border-radius: 12px;
    cursor: pointer; font-family: inherit; color: var(--text); font-size: .78rem; transition: border-color .15s; }
  .tool-chip:hover { border-color: var(--chip-color); }
  .tool-dot { width: 7px; height: 7px; border-radius: 50%; background: var(--chip-color); flex-shrink: 0; }
  .tool-name { font-weight: 500; }
  .tool-count { color: var(--text-muted); font-size: .72rem; }
  .tool-active { color: var(--success); font-size: .6rem; }

  /* Boards */
  .board-row { display: flex; align-items: center; justify-content: space-between; width: 100%;
    padding: .35rem .5rem; background: none; border: 1px solid var(--border); border-radius: var(--radius-sm);
    cursor: pointer; font-family: inherit; color: var(--text); text-align: left; margin-bottom: .3rem; transition: border-color .15s; }
  .board-row:hover { border-color: var(--primary); }
  .board-row-name { font-size: .82rem; font-weight: 600; }
  .board-row-stats { font-size: .72rem; color: var(--text-muted); display: flex; gap: .4rem; }
  .board-row-active { color: var(--success); }
  .in-progress-list { margin-top: .3rem; padding-top: .3rem; border-top: 1px solid var(--border); }
  .in-progress-item { display: flex; align-items: center; gap: .4rem; font-size: .75rem; color: var(--text-secondary); padding: .15rem 0; }
  .in-progress-dot { width: 5px; height: 5px; border-radius: 50%; background: var(--primary); flex-shrink: 0; }
  .in-progress-title { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }

  /* Recent */
  .recent-row { display: flex; align-items: center; gap: .5rem; width: 100%; padding: .3rem .5rem;
    background: none; border: none; cursor: pointer; font-family: inherit; color: var(--text);
    text-align: left; border-bottom: 1px solid var(--border); transition: background .15s; }
  .recent-row:last-child { border-bottom: none; }
  .recent-row:hover { background: var(--surface-hover); }
  .recent-dot { width: 6px; height: 6px; border-radius: 50%; flex-shrink: 0; }
  .recent-project { font-size: .8rem; font-weight: 600; flex-shrink: 0; max-width: 120px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .recent-summary { flex: 1; font-size: .75rem; color: var(--text-secondary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0; }
  .recent-badge { font-size: .6rem; color: var(--success); background: var(--success-dim); padding: .05rem .3rem; border-radius: 6px; flex-shrink: 0; }
  .recent-time { font-size: .7rem; color: var(--text-muted); flex-shrink: 0; }

  /* MCP hint */
  .mcp-hint { display: flex; align-items: center; justify-content: space-between; gap: 1rem;
    padding: .7rem 1rem; background: var(--primary-glow, rgba(99,102,241,.06));
    border: 1px solid var(--primary); border-radius: var(--radius-sm); margin-top: 1rem; flex-wrap: wrap; }
  .mcp-hint-content { flex: 1; font-size: .82rem; color: var(--text); min-width: 200px; }
  .mcp-hint-content strong { display: block; margin-bottom: .15rem; }
  .mcp-hint-content span { color: var(--text-secondary); }
  .mcp-hint-content code { font-size: .75rem; background: var(--surface); padding: .1rem .3rem; border-radius: 3px; }
  .mcp-hint-actions { display: flex; gap: .4rem; flex-shrink: 0; }

  /* Agents panel */
  .agents-panel { background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius); padding: .8rem; margin-bottom: 1rem; }
  .agents-panel.agents-active { border-color: var(--success); }
  .agents-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: .5rem; }
  .agents-title { font-size: .85rem; font-weight: 600; color: var(--success); margin: 0; }
  .agents-viewall {
    background: none; border: none; color: var(--primary); font-size: .75rem;
    cursor: pointer; font-family: inherit; text-decoration: underline;
  }
  .agent-row { display: flex; align-items: center; gap: .6rem; padding: .4rem 0; border-bottom: 1px solid var(--border); }
  .agent-row:last-child { border-bottom: none; }
  .agent-pulse { width: 8px; height: 8px; border-radius: 50%; background: var(--success); animation: pulse 2s infinite; flex-shrink: 0; }
  .agent-dot { width: 16px; height: 16px; font-size: .7rem; text-align: center; line-height: 16px; border-radius: 50%; flex-shrink: 0; }
  .agent-done { background: var(--success-dim, rgba(22,163,98,.15)); color: var(--success); }
  .agent-fail { background: var(--danger-dim, rgba(239,68,68,.15)); color: var(--danger); }
  .agent-info { flex: 1; min-width: 0; }
  .agent-task { display: block; font-size: .82rem; font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .agent-meta { display: block; font-size: .7rem; color: var(--text-muted); }
  .agent-log-panel { background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius); margin-bottom: 1rem; }
  .agent-log-header { display: flex; align-items: center; justify-content: space-between; padding: .5rem .8rem; border-bottom: 1px solid var(--border); font-size: .82rem; font-weight: 600; }
  .agent-log-header div { display: flex; gap: .3rem; }
  .agent-log-section { font-size: .72rem; font-weight: 600; color: var(--text-secondary); padding: .3rem .8rem; border-bottom: 1px solid var(--border); }
  .agent-log-content { font-size: .72rem; padding: .6rem .8rem; margin: 0; max-height: 200px; overflow-y: auto; white-space: pre-wrap; word-break: break-all; color: var(--text-secondary); background: var(--bg); }

  @media (max-width: 700px) {
    .dash-grid { grid-template-columns: 1fr; }
    .metrics { flex-wrap: wrap; }
    .metric-card { min-width: 70px; }
    .recent-summary { display: none; }
  }
</style>
