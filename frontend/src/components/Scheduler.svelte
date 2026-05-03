<script>
  import { fetchJobs, createJob, updateJob, deleteJob, triggerJob, pauseJob, resumeJob, cancelJob, fetchBoards, fetchSessions } from "../lib/api.js";

  let jobs = $state([]);
  let boards = $state([]);
  let loading = $state(true);
  let showEditor = $state(false);
  let editingJob = $state(null);
  let deleteConfirm = $state(null);
  let toast = $state("");

  let form = $state({
    name: "", cron: "0 9 * * *", tool: "claude", model: "",
    prompt: "", mcpServers: ["vibecockpit"], board: "", project: "",
    enabled: true, costCap: 0, permissions: ["read-files", "mcp-tools"],
  });

  let modelsByProvider = $state({});
  let allModels = $state([]);
  let knownProjects = $state([]);
  let customProject = $state(false);

  async function loadSessionData() {
    try {
      const sessions = await fetchSessions();
      const provModels = {};
      const allM = new Set();
      const seen = new Map();
      for (const s of sessions) {
        if (s.model && s.provider) {
          if (!provModels[s.provider]) provModels[s.provider] = new Set();
          provModels[s.provider].add(s.model);
          allM.add(s.model);
        }
        if (s.projectPath && !seen.has(s.projectPath)) {
          seen.set(s.projectPath, s.projectName || s.projectPath.split("/").pop());
        }
      }
      modelsByProvider = Object.fromEntries(
        Object.entries(provModels).map(([k, v]) => [k, [...v].sort()])
      );
      allModels = [...allM].sort();
      knownProjects = [...seen.entries()]
        .map(([path, name]) => ({ path, name }))
        .sort((a, b) => a.name.localeCompare(b.name));
    } catch { /* ignore */ }
  }

  function modelsForTool(tool) {
    if (!tool) return allModels;
    return modelsByProvider[tool] || allModels;
  }

  let allProjectPaths = $derived(() => {
    const paths = new Map();
    for (const p of knownProjects) {
      paths.set(p.path, p.name);
    }
    for (const b of boards) {
      if (b.project && !paths.has(b.project)) {
        paths.set(b.project, b.name);
      }
    }
    return [...paths.entries()]
      .map(([path, name]) => ({ path, name }))
      .sort((a, b) => a.name.localeCompare(b.name));
  });

  const cronPresets = [
    { label: "Every minute", value: "* * * * *" },
    { label: "Every 5 min", value: "*/5 * * * *" },
    { label: "Every 15 min", value: "*/15 * * * *" },
    { label: "Every hour", value: "0 * * * *" },
    { label: "Every 6 hours", value: "0 */6 * * *" },
    { label: "Daily at 9am", value: "0 9 * * *" },
    { label: "Daily at 6pm", value: "0 18 * * *" },
    { label: "Weekly Mon 9am", value: "0 9 * * 1" },
    { label: "Custom", value: "" },
  ];

  let selectedPreset = $state("0 9 * * *");

  async function load() {
    loading = true;
    try { jobs = await fetchJobs(); } catch { jobs = []; }
    loading = false;
  }

  async function loadBoards() {
    try { boards = await fetchBoards(); } catch { boards = []; }
  }

  load();
  loadSessionData();
  loadBoards();
  let pollInterval = setInterval(load, 10000);

  function openCreate() {
    editingJob = null;
    form = { name: "", cron: "0 9 * * *", tool: "claude", model: "", prompt: "", mcpServers: ["vibecockpit"], board: "", project: "", enabled: true, costCap: 0, permissions: ["read-files", "mcp-tools"] };
    selectedPreset = "0 9 * * *";
    customProject = false;
    showEditor = true;
  }

  function openEdit(job) {
    editingJob = job;
    form = { ...job, mcpServers: [...(job.mcpServers || ["vibecockpit"])], costCap: job.costCap || 0, permissions: [...(job.permissions || ["read-files", "mcp-tools"])] };
    selectedPreset = cronPresets.find(p => p.value === job.cron)?.value || "";
    customProject = job.project && !allProjectPaths().some(p => p.path === job.project);
    showEditor = true;
  }

  function onBoardChange(boardName) {
    form.board = boardName;
    if (boardName) {
      const b = boards.find(b => b.name === boardName);
      if (b && b.project && !form.project) {
        form.project = b.project;
      }
    }
  }

  async function saveJob() {
    try {
      const payload = { ...form };
      if (!payload.costCap) delete payload.costCap;
      if (editingJob) {
        await updateJob(editingJob.id, payload);
        showToast("Job updated");
      } else {
        await createJob(payload);
        showToast("Job created");
      }
      showEditor = false;
      await load();
    } catch (e) {
      showToast("Error: " + e.message);
    }
  }

  async function handleDelete(id) {
    try {
      await deleteJob(id);
      deleteConfirm = null;
      showToast("Job deleted");
      await load();
    } catch (e) { showToast("Error: " + e.message); }
  }

  async function handleTrigger(id) {
    try {
      await triggerJob(id);
      showToast("Job triggered");
      await load();
    } catch (e) { showToast("Error: " + e.message); }
  }

  async function handleToggle(job) {
    try {
      if (job.enabled) {
        await pauseJob(job.id);
        showToast("Job paused");
      } else {
        await resumeJob(job.id);
        showToast("Job resumed");
      }
      await load();
    } catch (e) { showToast("Error: " + e.message); }
  }

  async function handleCancel(id) {
    try {
      await cancelJob(id);
      showToast("Job cancelled");
      await load();
    } catch (e) { showToast("Error: " + e.message); }
  }

  function showToast(msg) {
    toast = msg;
    setTimeout(() => { toast = ""; }, 3000);
  }

  function timeUntil(iso) {
    if (!iso) return "";
    const diff = new Date(iso) - new Date();
    if (diff < 0) return "overdue";
    const h = Math.floor(diff / 3600000);
    const m = Math.floor((diff % 3600000) / 60000);
    if (h > 24) return `in ${Math.floor(h / 24)}d`;
    if (h > 0) return `in ${h}h ${m}m`;
    return `in ${m}m`;
  }

  function statusDot(status) {
    if (status === "running") return "dot-running";
    if (status === "completed") return "dot-completed";
    if (status === "failed" || status === "cancelled") return "dot-failed";
    return "dot-idle";
  }

  $effect(() => {
    return () => clearInterval(pollInterval);
  });
</script>

<div class="scheduler-page">
  <div class="page-bar">
    <div>
      <h2>Scheduler</h2>
      <span class="page-subtitle">Run agents on a schedule &mdash; {jobs.length} job{jobs.length !== 1 ? 's' : ''}</span>
    </div>
    <button class="btn-primary" onclick={openCreate}>+ New Job</button>
  </div>

  {#if loading && jobs.length === 0}
    <div class="empty-state">Loading jobs...</div>
  {:else if jobs.length === 0}
    <div class="empty-state">
      <p>No scheduled jobs yet.</p>
      <p class="hint">Create a job to run agents on a cron schedule — daily reports, board orchestration, dependency audits.</p>
      <button class="btn-primary" onclick={openCreate}>Create your first job</button>
    </div>
  {:else}
    <div class="job-list">
      {#each jobs as job (job.id)}
        <div class="job-card" class:disabled={!job.enabled}>
          <div class="job-top">
            <div class="job-header">
              <span class="job-status {statusDot(job.lastStatus)}"></span>
              <span class="job-name">{job.name}</span>
              <span class="job-tool">{job.tool}</span>
              {#if job.model}
                <span class="job-model">{job.model}</span>
              {/if}
            </div>
            <button
              class="toggle-switch"
              class:on={job.enabled}
              onclick={() => handleToggle(job)}
              role="switch"
              aria-checked={job.enabled}
              title={job.enabled ? 'Pause this job' : 'Resume this job'}
            >
              <span class="toggle-knob"></span>
            </button>
          </div>

          <div class="job-meta">
            <span class="job-cron" title={job.cron}>{job.humanCron || job.cron}</span>
            {#if job.enabled && job.nextRun}
              <span class="job-next">Next: {timeUntil(job.nextRun)}</span>
            {:else if !job.enabled}
              <span class="job-paused-label">Paused</span>
            {/if}
            {#if job.lastRun}
              <span class="job-last">Last: {new Date(job.lastRun).toLocaleString()}</span>
            {/if}
            {#if job.lastStatus && job.lastStatus !== "running"}
              <span class="job-last-status {job.lastStatus}">{job.lastStatus}</span>
            {/if}
            {#if job.project}
              <span class="job-project" title={job.project}>{job.project.split('/').pop()}</span>
            {/if}
            {#if job.board}
              <span class="job-board-tag">{job.board}</span>
            {/if}
          </div>

          {#if job.permissions?.length}
            <div class="job-perms">
              {#each job.permissions as p (p)}
                <span class="perm-tag">{p.replace('-', ' ')}</span>
              {/each}
            </div>
          {/if}

          <div class="job-prompt-preview">{job.prompt.length > 140 ? job.prompt.slice(0, 140) + '...' : job.prompt}</div>

          <div class="job-actions">
            <button class="btn-sm btn-run" onclick={() => handleTrigger(job.id)} disabled={job.lastStatus === 'running'}>
              {#if job.lastStatus === 'running'}Running...{:else}Run Now{/if}
            </button>
            {#if job.lastStatus === "running"}
              <button class="btn-sm btn-danger" onclick={() => handleCancel(job.id)}>Cancel</button>
            {/if}
            <button class="btn-sm" onclick={() => openEdit(job)}>Edit</button>
            <button class="btn-sm btn-danger-subtle" onclick={() => { deleteConfirm = job.id; }}>Delete</button>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>

{#if showEditor}
  <div class="modal-overlay" role="presentation" onclick={() => { showEditor = false; }} onkeydown={(e) => { if (e.key === 'Escape') showEditor = false; }} tabindex="-1">
    <div class="modal-content editor-modal" role="dialog" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={() => {}}>
      <h3>{editingJob ? 'Edit Job' : 'New Scheduled Job'}</h3>

      <div class="form-group">
        <label for="job-name">Name</label>
        <input id="job-name" type="text" bind:value={form.name} placeholder="e.g. Daily Board Review" />
      </div>

      <div class="form-group">
        <label for="job-cron">Schedule</label>
        <div class="cron-presets">
          {#each cronPresets as preset (preset.label)}
            <button
              class="preset-btn"
              class:active={selectedPreset === preset.value}
              onclick={() => { selectedPreset = preset.value; if (preset.value) form.cron = preset.value; }}
            >{preset.label}</button>
          {/each}
        </div>
        <input id="job-cron" type="text" bind:value={form.cron} placeholder="0 9 * * *" class="cron-input" />
        <span class="cron-hint">minute hour day month weekday</span>
      </div>

      <div class="form-row">
        <div class="form-group">
          <label for="job-tool">Tool</label>
          <select id="job-tool" bind:value={form.tool} onchange={() => { form.model = ''; }}>
            <option value="claude">Claude</option>
            <option value="codex">Codex</option>
            <option value="gemini">Gemini</option>
            <option value="opencode">OpenCode</option>
          </select>
        </div>
        <div class="form-group">
          <label for="job-model">Model</label>
          <select id="job-model" bind:value={form.model}>
            <option value="">Default</option>
            {#each modelsForTool(form.tool) as m (m)}
              <option value={m}>{m}</option>
            {/each}
          </select>
        </div>
      </div>

      <div class="form-group">
        <label for="job-prompt">Prompt</label>
        <textarea id="job-prompt" bind:value={form.prompt} rows="5" placeholder="What should the agent do on each run?"></textarea>
        <span class="field-hint">The global agent prompt from Settings is prepended automatically.</span>
      </div>

      <div class="form-section">
        <span class="section-title">Context</span>
      </div>

      <div class="form-group">
        <label for="job-project">Project directory</label>
        {#if customProject || allProjectPaths().length === 0}
          <input id="job-project" type="text" bind:value={form.project} placeholder="e.g. /home/user/projects/myapp" />
          {#if allProjectPaths().length > 0}
            <button class="link-btn" onclick={() => { customProject = false; }}>Choose from known projects</button>
          {/if}
        {:else}
          <select id="job-project" value={form.project} onchange={(e) => { form.project = e.target.value; }}>
            <option value="">Default workspace</option>
            {#each allProjectPaths() as p (p.path)}
              <option value={p.path}>{p.name} — {p.path}</option>
            {/each}
          </select>
          <button class="link-btn" onclick={() => { customProject = true; }}>Enter custom path</button>
        {/if}
        <span class="field-hint">The agent runs in this directory. Leave empty to use the default workspace.</span>
      </div>

      <div class="form-row">
        <div class="form-group">
          <label for="job-board">Board</label>
          <select id="job-board" value={form.board} onchange={(e) => onBoardChange(e.target.value)}>
            <option value="">None</option>
            {#each boards as b (b.name)}
              <option value={b.name}>{b.name}</option>
            {/each}
          </select>
          <span class="field-hint">Gives the agent access to board tasks via MCP.</span>
        </div>
        <div class="form-group">
          <label for="job-costcap">Cost cap ($)</label>
          <input id="job-costcap" type="number" bind:value={form.costCap} min="0" step="0.5" placeholder="0 = no limit" />
          <span class="field-hint">Skip the run if cumulative cost exceeds this limit.</span>
        </div>
      </div>

      <div class="form-section">
        <span class="section-title">Advanced</span>
      </div>

      <div class="form-group">
        <label for="job-mcp">MCP Servers</label>
        <input id="job-mcp" type="text" value={form.mcpServers.join(', ')} oninput={(e) => { form.mcpServers = e.target.value.split(',').map(s => s.trim()).filter(Boolean); }} placeholder="vibecockpit, postgres" />
        <span class="field-hint">Comma-separated. The agent gets access to these MCP servers during the run.</span>
      </div>

      <div class="form-group">
        <label>Allowed actions</label>
        <div class="perm-grid">
          {#each [
            { id: "read-files", label: "Read files", hint: "Read project files and code" },
            { id: "write-files", label: "Write / edit files", hint: "Create or modify files in the project" },
            { id: "run-commands", label: "Run shell commands", hint: "Execute build, test, and git commands" },
            { id: "mcp-tools", label: "Use MCP tools", hint: "Call tools from the configured MCP servers" },
            { id: "network", label: "Network / API access", hint: "Make HTTP requests or call external APIs" },
          ] as perm (perm.id)}
            <label class="perm-item">
              <input
                type="checkbox"
                checked={form.permissions.includes(perm.id)}
                onchange={(e) => {
                  if (e.target.checked) {
                    form.permissions = [...form.permissions, perm.id];
                  } else {
                    form.permissions = form.permissions.filter(p => p !== perm.id);
                  }
                }}
              />
              <span class="perm-label">{perm.label}</span>
              <span class="perm-hint">{perm.hint}</span>
            </label>
          {/each}
        </div>
        <span class="field-hint">Controls what the agent is allowed to do. Restrictive settings are safer for automated runs.</span>
      </div>

      <div class="form-group">
        <label class="toggle-row">
          <button
            class="toggle-switch"
            class:on={form.enabled}
            onclick={() => { form.enabled = !form.enabled; }}
            role="switch"
            aria-checked={form.enabled}
            type="button"
          >
            <span class="toggle-knob"></span>
          </button>
          <span>{form.enabled ? 'Job enabled — will run on schedule' : 'Job paused — will not run until enabled'}</span>
        </label>
      </div>

      <div class="modal-actions">
        <button class="btn-secondary" onclick={() => { showEditor = false; }}>Cancel</button>
        {#if editingJob}
          <button class="btn-run-now" onclick={async () => { await saveJob(); await handleTrigger(editingJob.id); }} disabled={!form.name || !form.cron || !form.prompt}>
            Save &amp; Run Now
          </button>
        {/if}
        <button class="btn-primary" onclick={saveJob} disabled={!form.name || !form.cron || !form.prompt}>
          {editingJob ? 'Save Changes' : 'Create Job'}
        </button>
      </div>
    </div>
  </div>
{/if}

{#if deleteConfirm}
  <div class="modal-overlay" role="presentation" onclick={() => { deleteConfirm = null; }} onkeydown={(e) => { if (e.key === 'Escape') deleteConfirm = null; }} tabindex="-1">
    <div class="modal-content delete-modal" role="dialog" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={() => {}}>
      <h3>Delete Job</h3>
      <p>Are you sure you want to delete this scheduled job? This cannot be undone.</p>
      <div class="modal-actions">
        <button class="btn-secondary" onclick={() => { deleteConfirm = null; }}>Cancel</button>
        <button class="btn-danger" onclick={() => handleDelete(deleteConfirm)}>Delete</button>
      </div>
    </div>
  </div>
{/if}

{#if toast}
  <div class="toast">{toast}</div>
{/if}

<style>
  .scheduler-page { padding: 0; }

  .page-bar {
    display: flex; justify-content: space-between; align-items: center;
    padding: 16px 24px; background: white; border-bottom: 1px solid #e5e7eb;
  }
  .page-bar h2 { margin: 0; font-size: 18px; font-weight: 600; }
  .page-subtitle { color: #6b7280; font-size: 13px; }

  .empty-state { text-align: center; padding: 80px 24px; color: #6b7280; }
  .empty-state p { margin: 8px 0; }
  .empty-state .hint { font-size: 13px; max-width: 400px; margin: 8px auto; }

  .job-list { padding: 16px 24px; display: flex; flex-direction: column; gap: 12px; }

  .job-card {
    background: white; border: 1px solid #e5e7eb; border-radius: 8px;
    padding: 16px; transition: box-shadow 0.15s;
  }
  .job-card:hover { box-shadow: 0 2px 8px rgba(0,0,0,0.06); }
  .job-card.disabled { opacity: 0.55; }

  .job-top { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 8px; }
  .job-header { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
  .job-name { font-weight: 600; font-size: 15px; }
  .job-tool {
    background: #f3f4f6; color: #374151; font-size: 11px; font-weight: 500;
    padding: 2px 8px; border-radius: 10px; text-transform: uppercase;
  }
  .job-model { color: #6b7280; font-size: 12px; }

  .job-status { width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; }
  .dot-running { background: #22c55e; animation: pulse 1.5s infinite; }
  .dot-completed { background: #22c55e; }
  .dot-failed { background: #ef4444; }
  .dot-idle { background: #d1d5db; }

  .job-meta {
    display: flex; gap: 12px; font-size: 13px; color: #6b7280;
    margin-bottom: 8px; flex-wrap: wrap; align-items: center;
  }
  .job-cron { font-family: monospace; background: #f9fafb; padding: 1px 6px; border-radius: 4px; font-size: 12px; }
  .job-next { color: #2563eb; font-weight: 500; }
  .job-paused-label { color: #9ca3af; font-style: italic; }
  .job-last-status { font-weight: 500; }
  .job-last-status.completed { color: #16a34a; }
  .job-last-status.failed, .job-last-status.cancelled { color: #dc2626; }
  .job-project {
    background: #f0fdf4; color: #15803d; font-size: 11px; padding: 1px 6px;
    border-radius: 4px; font-family: monospace;
  }
  .job-board-tag {
    background: #eff6ff; color: #2563eb; font-size: 11px; padding: 1px 6px;
    border-radius: 4px; font-weight: 500;
  }

  .job-perms { display: flex; gap: 4px; flex-wrap: wrap; margin-bottom: 6px; }
  .perm-tag {
    font-size: 10px; padding: 1px 6px; border-radius: 4px;
    background: #f3f4f6; color: #6b7280; text-transform: capitalize;
  }

  .job-prompt-preview {
    font-size: 13px; color: #4b5563; margin-bottom: 10px;
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }

  .job-actions { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }

  /* Toggle switch */
  .toggle-switch {
    position: relative; width: 36px; height: 20px; border-radius: 10px;
    background: #d1d5db; border: none; cursor: pointer; transition: background 0.2s;
    flex-shrink: 0; padding: 0;
  }
  .toggle-switch.on { background: #22c55e; }
  .toggle-knob {
    position: absolute; top: 2px; left: 2px; width: 16px; height: 16px;
    border-radius: 50%; background: white; transition: transform 0.2s;
    box-shadow: 0 1px 3px rgba(0,0,0,0.15);
  }
  .toggle-switch.on .toggle-knob { transform: translateX(16px); }

  .toggle-row { display: flex; align-items: center; gap: 10px; font-size: 13px; cursor: pointer; }

  .btn-primary {
    background: #2563eb; color: white; border: none; padding: 8px 16px;
    border-radius: 6px; font-size: 13px; cursor: pointer; font-weight: 500;
  }
  .btn-primary:hover { background: #1d4ed8; }
  .btn-primary:disabled { opacity: 0.5; cursor: not-allowed; }

  .btn-secondary {
    background: #f3f4f6; color: #374151; border: 1px solid #d1d5db;
    padding: 8px 16px; border-radius: 6px; font-size: 13px; cursor: pointer;
  }
  .btn-secondary:hover { background: #e5e7eb; }

  .btn-sm {
    background: #f3f4f6; color: #374151; border: 1px solid #e5e7eb;
    padding: 5px 12px; border-radius: 5px; font-size: 12px; cursor: pointer;
  }
  .btn-sm:hover { background: #e5e7eb; }
  .btn-sm:disabled { opacity: 0.5; cursor: not-allowed; }

  .btn-run { background: #eff6ff; color: #2563eb; border-color: #bfdbfe; font-weight: 500; }
  .btn-run:hover { background: #dbeafe; }

  .btn-danger { background: #dc2626; color: white; border: none; padding: 8px 16px; border-radius: 6px; font-size: 13px; cursor: pointer; }
  .btn-danger:hover { background: #b91c1c; }

  .btn-run-now {
    background: #eff6ff; color: #2563eb; border: 1px solid #bfdbfe;
    padding: 8px 16px; border-radius: 6px; font-size: 13px; cursor: pointer; font-weight: 500;
  }
  .btn-run-now:hover { background: #dbeafe; }
  .btn-run-now:disabled { opacity: 0.5; cursor: not-allowed; }

  .btn-danger-subtle { color: #dc2626; }
  .btn-danger-subtle:hover { background: #fef2f2; border-color: #fecaca; }

  /* Modal */
  .modal-overlay {
    position: fixed; inset: 0; background: rgba(0,0,0,0.4);
    display: flex; align-items: center; justify-content: center; z-index: 1000;
  }
  .modal-content {
    background: white; border-radius: 12px; padding: 24px;
    max-height: 90vh; overflow-y: auto;
  }
  .editor-modal { width: 560px; }
  .delete-modal { width: 400px; }
  .modal-content h3 { margin: 0 0 20px; font-size: 18px; }

  .form-section {
    margin: 18px 0 10px; padding-bottom: 4px;
    border-bottom: 1px solid #f3f4f6;
  }
  .section-title {
    font-size: 12px; font-weight: 600; text-transform: uppercase;
    letter-spacing: 0.5px; color: #9ca3af;
  }

  .form-group { margin-bottom: 14px; }
  .form-group label {
    display: block; font-size: 13px; font-weight: 500;
    color: #374151; margin-bottom: 4px;
  }
  .form-group input, .form-group select, .form-group textarea {
    width: 100%; padding: 8px 10px; border: 1px solid #d1d5db; border-radius: 6px;
    font-size: 14px; box-sizing: border-box;
  }
  .form-group textarea { font-family: monospace; font-size: 13px; resize: vertical; }
  .form-group input:focus, .form-group select:focus, .form-group textarea:focus {
    outline: none; border-color: #2563eb; box-shadow: 0 0 0 2px rgba(37, 99, 235, 0.1);
  }

  .field-hint { font-size: 11px; color: #9ca3af; margin-top: 3px; display: block; }
  .link-btn {
    background: none; border: none; color: #2563eb; font-size: 12px;
    cursor: pointer; padding: 2px 0; margin-top: 2px;
  }
  .link-btn:hover { text-decoration: underline; }

  .form-row { display: flex; gap: 12px; }
  .form-row .form-group { flex: 1; }

  .cron-presets { display: flex; flex-wrap: wrap; gap: 6px; margin-bottom: 8px; }
  .preset-btn {
    background: #f3f4f6; border: 1px solid #e5e7eb; padding: 4px 10px;
    border-radius: 14px; font-size: 12px; cursor: pointer; transition: all 0.15s;
  }
  .preset-btn:hover { background: #e5e7eb; }
  .preset-btn.active { background: #2563eb; color: white; border-color: #2563eb; }
  .cron-input { font-family: monospace; }
  .cron-hint { font-size: 11px; color: #9ca3af; font-family: monospace; }

  .perm-grid { display: flex; flex-direction: column; gap: 6px; margin-top: 4px; }
  .perm-item {
    display: grid; grid-template-columns: 18px auto; gap: 4px 8px; align-items: start;
    font-size: 13px; cursor: pointer; padding: 6px 8px; border-radius: 6px;
    border: 1px solid #f3f4f6; transition: background 0.1s;
  }
  .perm-item:hover { background: #f9fafb; }
  .perm-item input { margin: 2px 0 0; cursor: pointer; }
  .perm-label { font-weight: 500; color: #374151; }
  .perm-hint { grid-column: 2; font-size: 11px; color: #9ca3af; }

  .modal-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 20px; }

  .toast {
    position: fixed; bottom: 24px; left: 50%; transform: translateX(-50%);
    background: #1f2937; color: white; padding: 10px 20px; border-radius: 8px;
    font-size: 14px; z-index: 2000; animation: slideUp 0.3s;
  }

  @keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.4; }
  }
  @keyframes slideUp {
    from { transform: translateX(-50%) translateY(20px); opacity: 0; }
    to { transform: translateX(-50%) translateY(0); opacity: 1; }
  }
</style>
