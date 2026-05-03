<script>
  import { onMount, onDestroy } from "svelte";
  import { relativeTime } from "../lib/utils.js";

  let { onnavigate } = $props();
  let diffContent = $state("");
  let showDiff = $state(false);
  let deleteConfirm = $state({ open: false, taskId: "", cleanBranch: false });

  let agents = $state([]);
  let selectedAgent = $state(null);
  let agentLog = $state({ stdout: "", status: "", recent: "" });
  let parentJob = $state(null);
  let logAutoRefresh = $state(true);
  let searchQuery = $state("");
  let filterSource = $state("all");
  let filterStatus = $state("all");
  let pollTimer;
  let logTimer;

  let filteredAgents = $derived(() => {
    let list = [...agents];

    list.sort((a, b) => new Date(b.startedAt) - new Date(a.startedAt));

    if (filterSource === "scheduled") list = list.filter(a => a.source === "scheduled");
    else if (filterSource === "task") list = list.filter(a => a.source !== "scheduled");

    if (filterStatus === "running") list = list.filter(a => a.status === "running");
    else if (filterStatus === "completed") list = list.filter(a => a.status === "completed");
    else if (filterStatus === "failed") list = list.filter(a => a.status === "failed" || a.status === "cancelled");

    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      list = list.filter(a =>
        (a.taskTitle || "").toLowerCase().includes(q) ||
        (a.taskId || "").toLowerCase().includes(q) ||
        (a.boardName || "").toLowerCase().includes(q) ||
        (a.project || "").toLowerCase().includes(q) ||
        (a.tool || "").toLowerCase().includes(q) ||
        (a.model || "").toLowerCase().includes(q)
      );
    }

    return list;
  });

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

  function jobIdFromTaskId(taskId) {
    return taskId.startsWith("job-") ? taskId.slice(4) : null;
  }

  function siblingRuns(taskId) {
    return agents
      .filter(a => a.taskId === taskId || (a.source === "scheduled" && a.taskId.slice(4) === taskId.slice(4)))
      .sort((a, b) => new Date(b.startedAt) - new Date(a.startedAt));
  }

  async function loadParentJob(taskId) {
    const jobId = jobIdFromTaskId(taskId);
    if (!jobId) { parentJob = null; return; }
    try {
      const r = await fetch(`/api/jobs/${encodeURIComponent(jobId)}`);
      if (r.ok) parentJob = await r.json();
      else parentJob = null;
    } catch { parentJob = null; }
  }

  function selectAgent(run) {
    selectedAgent = run;
    loadLog(run.taskId);
    if (run.source === "scheduled") loadParentJob(run.taskId);
    else parentJob = null;
    if (logTimer) clearInterval(logTimer);
    if (run.status === "running" && logAutoRefresh) {
      logTimer = setInterval(() => loadLog(run.taskId), 3000);
    }
  }

  async function viewDiff(taskId) {
    try {
      const r = await fetch(`/api/agents/${encodeURIComponent(taskId)}/diff`);
      if (r.ok) {
        const d = await r.json();
        diffContent = d.diff || "(no changes)";
        showDiff = true;
      }
    } catch { diffContent = "(failed to load diff)"; showDiff = true; }
  }

  async function mergeAgent(taskId) {
    if (!confirm("Merge this agent's branch into main?")) return;
    try {
      const r = await fetch(`/api/agents/${encodeURIComponent(taskId)}/merge`, { method: "POST" });
      if (r.ok) {
        loadAgents();
        alert("Branch merged successfully");
      } else {
        const d = await r.json();
        alert("Merge failed: " + (d.error || "unknown error"));
      }
    } catch { alert("Merge failed"); }
  }

  function showDeleteConfirm(taskId) {
    deleteConfirm = { open: true, taskId, cleanBranch: false };
  }

  async function doDelete() {
    const { taskId, cleanBranch } = deleteConfirm;
    try {
      await fetch(`/api/agents/${encodeURIComponent(taskId)}?branch=${cleanBranch}`, { method: "DELETE" });
      if (selectedAgent?.taskId === taskId) selectedAgent = null;
      deleteConfirm = { open: false, taskId: "", cleanBranch: false };
      loadAgents();
    } catch { /* ignore */ }
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

        <input
          class="agent-search"
          type="text"
          placeholder="Search runs..."
          bind:value={searchQuery}
        />

        <div class="agent-filters">
          <select class="agent-filter-select" bind:value={filterSource}>
            <option value="all">All sources</option>
            <option value="task">Task runs</option>
            <option value="scheduled">Scheduled</option>
          </select>
          <select class="agent-filter-select" bind:value={filterStatus}>
            <option value="all">All statuses</option>
            <option value="running">Running</option>
            <option value="completed">Completed</option>
            <option value="failed">Failed</option>
          </select>
        </div>

        {#if filteredAgents().length === 0}
          <div class="agent-no-match">No matching runs</div>
        {/if}

        {#each filteredAgents() as run (run.taskId)}
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
              <span class="agent-item-task">
                {run.taskTitle || run.taskId}
                {#if run.source === "scheduled"}
                  <span class="source-tag source-scheduled" title="Triggered by a scheduled job">Scheduled</span>
                {/if}
              </span>
              <span class="agent-item-meta">
                {run.boardName || run.project?.split('/').pop() || ''} · {run.tool}{run.model ? " · " + run.model : ""}
              </span>
            </div>
            <div class="agent-item-right">
              {#if run.status === "running"}
                <span class="agent-item-elapsed">{run.elapsed}</span>
              {:else}
                <span class="agent-item-status-text">{run.status}</span>
              {/if}
              <span class="agent-item-time">{relativeTime(run.startedAt)}</span>
            </div>
          </button>
        {/each}
      </div>

      <!-- Agent detail -->
      <div class="agent-detail">
        {#if selectedAgent}
          <div class="agent-detail-header">
            <div>
              <h3 class="agent-detail-title">
                {selectedAgent.taskTitle || selectedAgent.taskId}
                {#if selectedAgent.source === "scheduled"}
                  <span class="source-tag source-scheduled">Scheduled</span>
                {/if}
              </h3>
              <span class="agent-detail-meta">
                {#if selectedAgent.boardName}
                  <button class="agent-link" onclick={() => onnavigate("planner")}>{selectedAgent.boardName}</button> ·
                {/if}
                {selectedAgent.project} · {selectedAgent.tool}{selectedAgent.model ? " · " + selectedAgent.model : ""}
                {#if selectedAgent.source === "scheduled"}
                  · <button class="agent-link" onclick={() => onnavigate("scheduler")}>View schedule</button>
                {/if}
              </span>
            </div>
            <div class="agent-detail-actions">
              {#if selectedAgent.status !== "running"}
                <button class="btn btn-sm" onclick={() => viewDiff(selectedAgent.taskId)}>View diff</button>
                <button class="btn btn-sm btn-primary" onclick={() => mergeAgent(selectedAgent.taskId)}>Merge</button>
              {/if}
              {#if selectedAgent.status === "running"}
                <button class="btn btn-sm btn-danger" onclick={() => stopAgent(selectedAgent.taskId)}>Stop</button>
              {:else}
                <button class="btn btn-sm btn-danger" onclick={() => showDeleteConfirm(selectedAgent.taskId)}>Delete</button>
              {/if}
              <button class="btn btn-sm" onclick={() => loadLog(selectedAgent.taskId)}>Refresh</button>
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

          {#if parentJob && selectedAgent?.source === "scheduled"}
            <div class="parent-job-panel">
              <div class="parent-job-header">
                <span class="parent-job-title">Job: {parentJob.name}</span>
                <button class="agent-link" onclick={() => onnavigate("scheduler")}>Edit</button>
              </div>
              <div class="parent-job-config">
                <div class="parent-job-field">
                  <span class="parent-job-label">Schedule</span>
                  <span>{parentJob.humanCron || parentJob.cron}</span>
                  <code class="parent-job-cron">{parentJob.cron}</code>
                </div>
                <div class="parent-job-field">
                  <span class="parent-job-label">Tool / Model</span>
                  <span>{parentJob.tool}{parentJob.model ? " / " + parentJob.model : ""}</span>
                </div>
                {#if parentJob.project}
                  <div class="parent-job-field">
                    <span class="parent-job-label">Project</span>
                    <span class="parent-job-mono">{parentJob.project}</span>
                  </div>
                {/if}
                {#if parentJob.mcpServers?.length}
                  <div class="parent-job-field">
                    <span class="parent-job-label">MCP</span>
                    <span>{parentJob.mcpServers.join(", ")}</span>
                  </div>
                {/if}
                {#if parentJob.costCap}
                  <div class="parent-job-field">
                    <span class="parent-job-label">Cost cap</span>
                    <span>${parentJob.costCap}</span>
                  </div>
                {/if}
                <div class="parent-job-field parent-job-prompt">
                  <span class="parent-job-label">Prompt</span>
                  <span>{parentJob.prompt.length > 200 ? parentJob.prompt.slice(0, 200) + "..." : parentJob.prompt}</span>
                </div>
              </div>

              {#if siblingRuns(selectedAgent.taskId).length > 1}
                <div class="parent-job-runs">
                  <span class="parent-job-label">Run history ({siblingRuns(selectedAgent.taskId).length})</span>
                  <div class="sibling-list">
                    {#each siblingRuns(selectedAgent.taskId) as sib (sib.taskId + sib.startedAt)}
                      <button
                        class="sibling-item"
                        class:sibling-active={sib.taskId === selectedAgent.taskId && sib.startedAt === selectedAgent.startedAt}
                        onclick={() => selectAgent(sib)}
                      >
                        <span class="sibling-dot" class:dot-ok={sib.status === "completed" || sib.status === "running"} class:dot-fail={sib.status === "failed" || sib.status === "cancelled"}></span>
                        <span class="sibling-time">{relativeTime(sib.startedAt)}</span>
                        <span class="sibling-status">{sib.status}{sib.elapsed ? " · " + sib.elapsed : ""}</span>
                      </button>
                    {/each}
                  </div>
                </div>
              {/if}
            </div>
          {/if}

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

          {#if showDiff}
            <div class="agent-log">
              <div class="agent-log-bar">
                <span>Git diff (branch: vibecockpit/{selectedAgent.taskId})</span>
                <button class="btn btn-sm" onclick={() => { showDiff = false; }}>Close</button>
              </div>
              <pre class="agent-log-output agent-log-diff">{diffContent}</pre>
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

{#if deleteConfirm.open}
<div class="agent-overlay" onclick={() => { deleteConfirm = { open: false, taskId: "", cleanBranch: false }; }} onkeydown={(e) => { if (e.key === 'Escape') deleteConfirm = { open: false, taskId: "", cleanBranch: false }; }} role="dialog" aria-modal="true" tabindex="-1">
  <div class="agent-modal" onclick={(e) => e.stopPropagation()} role="presentation">
    <h3>Delete agent run</h3>
    <p style="font-size:.85rem;color:var(--text-secondary);margin:.5rem 0 1rem">
      Remove <strong>{deleteConfirm.taskId}</strong> from the agent runs list.
    </p>
    <label style="display:flex;align-items:center;gap:.5rem;font-size:.85rem;margin-bottom:1rem;cursor:pointer">
      <input type="checkbox" checked={deleteConfirm.cleanBranch} onchange={(e) => { deleteConfirm = { ...deleteConfirm, cleanBranch: e.target.checked }; }} />
      Also delete git branch <code>vibecockpit/{deleteConfirm.taskId}</code>
    </label>
    <div style="display:flex;gap:.4rem;justify-content:flex-end">
      <button class="btn btn-sm" onclick={() => { deleteConfirm = { open: false, taskId: "", cleanBranch: false }; }}>Cancel</button>
      <button class="btn btn-sm btn-danger" onclick={doDelete}>Delete</button>
    </div>
  </div>
</div>
{/if}

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

  .agent-search {
    width: 100%; padding: .4rem .6rem; border: 1px solid var(--border); border-radius: var(--radius-sm);
    font-size: .8rem; margin-bottom: .4rem; box-sizing: border-box; background: var(--surface);
    color: var(--text);
  }
  .agent-search:focus { outline: none; border-color: var(--primary); }
  .agent-search::placeholder { color: var(--text-muted); }

  .agent-filters { display: flex; gap: .3rem; margin-bottom: .5rem; }
  .agent-filter-select {
    flex: 1; padding: .3rem .4rem; border: 1px solid var(--border); border-radius: var(--radius-sm);
    font-size: .72rem; background: var(--surface); color: var(--text); cursor: pointer;
  }

  .agent-no-match { text-align: center; padding: 1.5rem .5rem; font-size: .82rem; color: var(--text-muted); }

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
  .agent-item-time { display: block; font-size: .65rem; color: var(--text-muted); margin-top: 1px; }

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
  .agent-log-diff { font-family: monospace; font-size: .7rem; max-height: 400px; }
  .agent-link { background: none; border: none; color: var(--primary); cursor: pointer; font: inherit; padding: 0; text-decoration: underline; }

  .source-tag {
    display: inline-block; font-size: .6rem; font-weight: 600; padding: 1px 5px;
    border-radius: 6px; vertical-align: middle; margin-left: 4px; text-transform: uppercase; letter-spacing: .3px;
  }
  .source-scheduled { background: #fef3c7; color: #92400e; }

  /* Parent job panel */
  .parent-job-panel {
    background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius);
    padding: .8rem; margin-bottom: .5rem;
  }
  .parent-job-header {
    display: flex; justify-content: space-between; align-items: center; margin-bottom: .5rem;
  }
  .parent-job-title { font-size: .85rem; font-weight: 700; }
  .parent-job-config { display: flex; flex-wrap: wrap; gap: .3rem .8rem; }
  .parent-job-field { font-size: .78rem; }
  .parent-job-label {
    display: inline-block; font-size: .65rem; font-weight: 600; color: var(--text-muted);
    text-transform: uppercase; letter-spacing: .3px; margin-right: .3rem;
  }
  .parent-job-cron {
    font-size: .68rem; background: var(--bg); padding: .1rem .3rem; border-radius: 3px;
    margin-left: .3rem; color: var(--text-muted);
  }
  .parent-job-mono { font-family: monospace; font-size: .75rem; }
  .parent-job-prompt {
    flex-basis: 100%; margin-top: .3rem; font-size: .75rem; color: var(--text-secondary);
    white-space: pre-wrap; line-height: 1.4;
  }

  .parent-job-runs { margin-top: .6rem; border-top: 1px solid var(--border); padding-top: .5rem; }
  .sibling-list { display: flex; flex-direction: column; gap: .2rem; margin-top: .3rem; max-height: 150px; overflow-y: auto; }
  .sibling-item {
    display: flex; align-items: center; gap: .4rem; padding: .25rem .4rem;
    background: none; border: 1px solid transparent; border-radius: var(--radius-sm);
    font-family: inherit; font-size: .75rem; color: var(--text); cursor: pointer;
    text-align: left; width: 100%;
  }
  .sibling-item:hover { background: var(--bg); }
  .sibling-active { border-color: var(--primary); background: var(--primary-glow, rgba(99,102,241,.06)); }
  .sibling-dot { width: 6px; height: 6px; border-radius: 50%; flex-shrink: 0; background: var(--text-muted); }
  .sibling-dot.dot-ok { background: var(--success); }
  .sibling-dot.dot-fail { background: var(--danger); }
  .sibling-time { color: var(--text-muted); font-size: .7rem; }
  .sibling-status { font-size: .7rem; color: var(--text-secondary); }

  .agent-overlay { position: fixed; inset: 0; z-index: 100; background: rgba(0,0,0,.4); display: flex; align-items: center; justify-content: center; }
  .agent-modal { background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius); padding: 1.5rem; width: 90%; max-width: 400px; }
  .agent-modal h3 { font-size: 1rem; margin: 0; }
  .agent-modal code { font-size: .78rem; background: var(--bg); padding: .1rem .3rem; border-radius: 3px; }
  .btn-danger { color: var(--danger) !important; }
  .btn-danger:hover { background: var(--danger-dim, rgba(239,68,68,.1)); }

  @media (max-width: 800px) {
    .agent-layout { flex-direction: column; }
    .agent-list { width: 100%; }
  }
</style>
