<script>
  import { onMount, onDestroy } from "svelte";
  import { relativeTime } from "../lib/utils.js";

  let agents = $state([]);
  let selectedAgent = $state(null);
  let agentLog = $state({ stdout: "", status: "", recent: "" });
  let logAutoRefresh = $state(true);
  let pollTimer;
  let logTimer;

  async function loadAgents() {
    try {
      const r = await fetch("/api/agents");
      if (r.ok) {
        agents = await r.json();
        if (selectedAgent) {
          const updated = agents.find(a => a.taskId === selectedAgent.taskId);
          if (updated) selectedAgent = updated;
        }
      }
    } catch { /* ignore */ }
  }

  async function loadLog(taskId) {
    try {
      const r = await fetch(`/api/agents/${encodeURIComponent(taskId)}/log`);
      if (r.ok) {
        agentLog = await r.json();
      }
    } catch { agentLog = { stdout: "(failed to load)", status: "", recent: "" }; }
  }

  function selectAgent(run) {
    selectedAgent = run;
    loadLog(run.taskId);
    if (logTimer) clearInterval(logTimer);
    if (run.status === "running" && logAutoRefresh) {
      logTimer = setInterval(() => loadLog(run.taskId), 3000);
    }
  }

  async function stopAgent(taskId) {
    try {
      await fetch(`/api/agents/${encodeURIComponent(taskId)}/stop`, { method: "POST" });
      setTimeout(loadAgents, 1000);
    } catch { /* ignore */ }
  }

  let runningCount = $derived(agents.filter(a => a.status === "running").length);
  let completedCount = $derived(agents.filter(a => a.status === "completed").length);
  let failedCount = $derived(agents.filter(a => a.status === "failed").length);

  onMount(() => {
    loadAgents();
    pollTimer = setInterval(() => {
      loadAgents();
      if (selectedAgent?.status === "running") {
        loadLog(selectedAgent.taskId);
      }
    }, 5000);
  });

  onDestroy(() => {
    clearInterval(pollTimer);
    clearInterval(logTimer);
  });
</script>

<div class="agent-page">
  {#if agents.length === 0}
    <div class="agent-empty">
      <p style="font-size:1.1rem;font-weight:600;margin-bottom:.5rem">No agents spawned yet</p>
      <p style="font-size:.85rem;color:var(--text-secondary)">
        Go to the Planner, open a task with a tool configured, and click "Run" to spawn an agent.
        Or use the CLI: <code>vibecockpit run &lt;task-id&gt;</code>
      </p>
    </div>
  {:else}
    <div class="agent-layout">
      <!-- Agent list -->
      <div class="agent-list">
        <div class="agent-list-stats">
          {#if runningCount > 0}<span class="agent-stat agent-stat-running">{runningCount} running</span>{/if}
          {#if completedCount > 0}<span class="agent-stat agent-stat-done">{completedCount} completed</span>{/if}
          {#if failedCount > 0}<span class="agent-stat agent-stat-fail">{failedCount} failed</span>{/if}
        </div>
        {#each agents as run (run.taskId)}
          <button class="agent-item" class:agent-item-active={selectedAgent?.taskId === run.taskId} onclick={() => selectAgent(run)}>
            <div class="agent-item-status">
              {#if run.status === "running"}
                <span class="agent-pulse-sm"></span>
              {:else if run.status === "completed"}
                <span class="agent-icon-done">&#10003;</span>
              {:else}
                <span class="agent-icon-fail">&#10005;</span>
              {/if}
            </div>
            <div class="agent-item-info">
              <span class="agent-item-task">{run.taskTitle || run.taskId}</span>
              <span class="agent-item-meta">
                {run.boardName} · {run.tool}{run.model ? " · " + run.model : ""}
              </span>
            </div>
            <div class="agent-item-right">
              {#if run.status === "running"}
                <span class="agent-item-elapsed">{run.elapsed}</span>
              {:else}
                <span class="agent-item-status-text">{run.status}</span>
              {/if}
            </div>
          </button>
        {/each}
      </div>

      <!-- Agent detail -->
      <div class="agent-detail">
        {#if selectedAgent}
          <div class="agent-detail-header">
            <div>
              <h3 class="agent-detail-title">{selectedAgent.taskTitle || selectedAgent.taskId}</h3>
              <span class="agent-detail-meta">
                {selectedAgent.boardName} · {selectedAgent.project} · {selectedAgent.tool}{selectedAgent.model ? " · " + selectedAgent.model : ""}
              </span>
            </div>
            <div class="agent-detail-actions">
              {#if selectedAgent.status === "running"}
                <button class="btn btn-sm btn-danger" onclick={() => stopAgent(selectedAgent.taskId)}>Stop</button>
              {/if}
              <button class="btn btn-sm" onclick={() => loadLog(selectedAgent.taskId)}>Refresh log</button>
            </div>
          </div>

          <div class="agent-detail-stats">
            <div class="agent-detail-stat">
              <span class="agent-detail-stat-label">Status</span>
              <span class="agent-detail-stat-value" class:text-success={selectedAgent.status === "running" || selectedAgent.status === "completed"} class:text-danger={selectedAgent.status === "failed"}>
                {selectedAgent.status}
              </span>
            </div>
            <div class="agent-detail-stat">
              <span class="agent-detail-stat-label">Started</span>
              <span class="agent-detail-stat-value">{relativeTime(selectedAgent.startedAt)}</span>
            </div>
            {#if selectedAgent.elapsed}
              <div class="agent-detail-stat">
                <span class="agent-detail-stat-label">Elapsed</span>
                <span class="agent-detail-stat-value">{selectedAgent.elapsed}</span>
              </div>
            {/if}
            <div class="agent-detail-stat">
              <span class="agent-detail-stat-label">PID</span>
              <span class="agent-detail-stat-value">{selectedAgent.pid}</span>
            </div>
            {#if selectedAgent.exitCode}
              <div class="agent-detail-stat">
                <span class="agent-detail-stat-label">Exit code</span>
                <span class="agent-detail-stat-value">{selectedAgent.exitCode}</span>
              </div>
            {/if}
          </div>

          {#if agentLog.status}
            <div class="agent-log">
              <div class="agent-log-bar">
                <span>STATUS.md</span>
              </div>
              <pre class="agent-log-output agent-log-status">{agentLog.status}</pre>
            </div>
          {/if}

          {#if agentLog.recent}
            <div class="agent-log">
              <div class="agent-log-bar">
                <span>Recent agent activity</span>
              </div>
              <pre class="agent-log-output">{agentLog.recent}</pre>
            </div>
          {/if}

          <div class="agent-log">
            <div class="agent-log-bar">
              <span>Stdout / stderr</span>
              {#if selectedAgent.logPath}
                <code class="agent-log-path">{selectedAgent.logPath}</code>
              {/if}
            </div>
            <pre class="agent-log-output">{agentLog.stdout || "(empty)"}</pre>
          </div>
        {:else}
          <div class="agent-detail-empty">Select an agent to view details and output log</div>
        {/if}
      </div>
    </div>
  {/if}
</div>

<style>
  .agent-page { max-width: 1200px; margin: 0 auto; padding: 0 1.5rem 2rem; }
  .agent-empty { text-align: center; padding: 3rem 1rem; background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius); }

  .agent-layout { display: flex; gap: 1rem; min-height: 500px; }

  /* List */
  .agent-list { width: 280px; flex-shrink: 0; }
  .agent-list-stats { display: flex; gap: .5rem; margin-bottom: .5rem; flex-wrap: wrap; }
  .agent-stat { font-size: .72rem; font-weight: 600; padding: .15rem .4rem; border-radius: 8px; }
  .agent-stat-running { color: var(--success); background: var(--success-dim, rgba(22,163,98,.1)); }
  .agent-stat-done { color: var(--text-muted); background: var(--bg); }
  .agent-stat-fail { color: var(--danger); background: var(--danger-dim, rgba(239,68,68,.1)); }

  .agent-item { display: flex; align-items: center; gap: .5rem; width: 100%; padding: .5rem .6rem;
    background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius-sm);
    cursor: pointer; font-family: inherit; color: var(--text); text-align: left; margin-bottom: .3rem; transition: border-color .15s; }
  .agent-item:hover { border-color: var(--primary); }
  .agent-item-active { border-color: var(--primary); background: var(--primary-glow, rgba(99,102,241,.06)); }

  .agent-item-status { width: 16px; flex-shrink: 0; text-align: center; }
  .agent-pulse-sm { display: inline-block; width: 8px; height: 8px; border-radius: 50%; background: var(--success); animation: pulse 2s infinite; }
  @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: .4; } }
  .agent-icon-done { color: var(--success); font-size: .75rem; }
  .agent-icon-fail { color: var(--danger); font-size: .75rem; }

  .agent-item-info { flex: 1; min-width: 0; }
  .agent-item-task { display: block; font-size: .82rem; font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .agent-item-meta { display: block; font-size: .68rem; color: var(--text-muted); }
  .agent-item-right { flex-shrink: 0; text-align: right; }
  .agent-item-elapsed { font-size: .75rem; font-weight: 600; color: var(--success); }
  .agent-item-status-text { font-size: .72rem; color: var(--text-muted); }

  /* Detail */
  .agent-detail { flex: 1; min-width: 0; }
  .agent-detail-empty { padding: 3rem 1rem; text-align: center; color: var(--text-muted); background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius); }
  .agent-detail-header { display: flex; align-items: flex-start; justify-content: space-between; margin-bottom: .8rem; }
  .agent-detail-title { font-size: 1rem; font-weight: 700; margin: 0; }
  .agent-detail-meta { font-size: .78rem; color: var(--text-muted); }
  .agent-detail-actions { display: flex; gap: .3rem; }

  .agent-detail-stats { display: flex; gap: 1rem; margin-bottom: .8rem; flex-wrap: wrap; }
  .agent-detail-stat { background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius-sm); padding: .4rem .6rem; }
  .agent-detail-stat-label { display: block; font-size: .65rem; color: var(--text-muted); text-transform: uppercase; letter-spacing: .5px; }
  .agent-detail-stat-value { font-size: .85rem; font-weight: 600; }
  .text-success { color: var(--success); }
  .text-danger { color: var(--danger); }

  .agent-log { background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius); overflow: hidden; }
  .agent-log-bar { display: flex; align-items: center; justify-content: space-between; padding: .4rem .8rem; border-bottom: 1px solid var(--border); font-size: .78rem; font-weight: 600; }
  .agent-log-path { font-size: .68rem; color: var(--text-muted); }
  .agent-log-output { font-size: .72rem; padding: .6rem .8rem; margin: 0; max-height: 300px; overflow-y: auto; white-space: pre-wrap; word-break: break-all; color: var(--text-secondary); background: var(--bg); }
  .agent-log-status { color: var(--text); font-size: .78rem; }
  .agent-log + .agent-log { margin-top: .5rem; }

  @media (max-width: 800px) {
    .agent-layout { flex-direction: column; }
    .agent-list { width: 100%; }
  }
</style>
