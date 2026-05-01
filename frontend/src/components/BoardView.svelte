<script>
  import { onMount } from "svelte";
  import { fetchBoards, fetchBoard, createBoard, addBoardTask, updateBoardTask, deleteBoardTask, deleteBoard, moveTaskToBoard, fetchConfig, fetchSessions } from "../lib/api.js";
  import { relativeTime, providerColors } from "../lib/utils.js";

  let boards = $state([]);
  let activeBoard = $state(null);
  let loading = $state(true);
  let showCreateBoard = $state(false);
  let showAddTask = $state(false);
  let selectedTask = $state(null);
  let availableProviders = $state([]);
  let modelsByProvider = $state({});
  let allModels = $state([]);
  let knownProjects = $state([]);
  let customProject = $state(false);

  function modelsForTool(tool) {
    if (!tool) return allModels;
    return modelsByProvider[tool] || allModels;
  }

  let newBoardName = $state("");
  let newBoardProject = $state("");
  let newTaskTitle = $state("");
  let newTaskPriority = $state("medium");
  let newTaskDescription = $state("");
  let newTaskTool = $state("");
  let newTaskModel = $state("");

  let dragTask = $state(null);
  let dragOverCol = $state(null);
  let showArchived = $state(false);
  let filterPriority = $state("");

  const defaultColumns = ["backlog", "claimed", "in-progress", "review", "done"];

  const columnLabels = {
    "backlog": "Backlog",
    "claimed": "Claimed",
    "in-progress": "Working",
    "review": "Review",
    "done": "Done",
  };

  const priorityColors = {
    "high": "var(--danger)",
    "medium": "var(--warning, #f59e0b)",
    "low": "var(--text-muted)",
  };

  function columns() {
    return activeBoard?.columns?.length ? activeBoard.columns : defaultColumns;
  }

  function tasksByColumn(col) {
    return (activeBoard?.tasks || []).filter(t => {
      if (t.status !== col || t.status === "archived") return false;
      if (filterPriority && t.priority !== filterPriority) return false;
      return true;
    });
  }

  function totalCost() {
    return (activeBoard?.tasks || [])
      .filter(t => t.cost)
      .reduce((sum, t) => sum + parseFloat(t.cost.replace("$", "") || 0), 0);
  }

  async function load() {
    try {
      boards = await fetchBoards();
      if (boards.length > 0 && !activeBoard) {
        activeBoard = boards[0];
      } else if (activeBoard) {
        const fresh = boards.find(b => b.name === activeBoard.name);
        if (fresh) activeBoard = fresh;
      }
    } catch { /* ignore */ }
    loading = false;
  }

  async function loadConfig() {
    try {
      const cfg = await fetchConfig();
      availableProviders = (cfg.allProviders || []).map(p => p.id);
    } catch { /* ignore */ }
    try {
      const sessions = await fetchSessions();
      const seen = new Map();
      const provModels = {};
      const allM = new Set();
      for (const s of sessions) {
        if (s.projectPath && !seen.has(s.projectPath)) {
          seen.set(s.projectPath, s.projectName || s.projectPath.split("/").pop());
        }
        if (s.model && s.provider) {
          if (!provModels[s.provider]) provModels[s.provider] = new Set();
          provModels[s.provider].add(s.model);
          allM.add(s.model);
        }
      }
      knownProjects = [...seen.entries()]
        .map(([path, name]) => ({ path, name }))
        .sort((a, b) => a.name.localeCompare(b.name));
      modelsByProvider = Object.fromEntries(
        Object.entries(provModels).map(([k, v]) => [k, [...v].sort()])
      );
      allModels = [...allM].sort();
    } catch { /* ignore */ }
  }

  async function selectBoard(name) {
    try {
      activeBoard = await fetchBoard(name);
    } catch { /* ignore */ }
  }

  async function doCreateBoard() {
    if (!newBoardName.trim()) return;
    try {
      await createBoard(newBoardName.trim(), newBoardProject.trim() || ".");
      showCreateBoard = false;
      customProject = false;
      newBoardName = "";
      newBoardProject = "";
      await load();
      await selectBoard(newBoardName.trim() || boards[boards.length - 1]?.name);
    } catch { /* ignore */ }
  }

  async function doAddTask() {
    if (!newTaskTitle.trim() || !activeBoard) return;
    try {
      await addBoardTask(activeBoard.name, newTaskTitle.trim(), newTaskPriority, newTaskDescription.trim(), newTaskTool, newTaskModel);
      showAddTask = false;
      newTaskTitle = "";
      newTaskPriority = "medium";
      newTaskDescription = "";
      newTaskTool = "";
      newTaskModel = "";
      await selectBoard(activeBoard.name);
    } catch { /* ignore */ }
  }

  async function moveTask(taskId, newStatus) {
    if (!activeBoard) return;
    try {
      await updateBoardTask(activeBoard.name, taskId, { status: newStatus });
      await selectBoard(activeBoard.name);
    } catch { /* ignore */ }
  }

  async function removeBoard(name) {
    if (!confirm(`Delete board "${name}"? This removes the YAML file permanently.`)) return;
    try {
      await deleteBoard(name);
      if (activeBoard?.name === name) activeBoard = null;
      await load();
    } catch { /* ignore */ }
  }

  async function archiveTask(taskId) {
    if (!activeBoard) return;
    try {
      await updateBoardTask(activeBoard.name, taskId, { status: "archived" });
      selectedTask = null;
      await selectBoard(activeBoard.name);
    } catch { /* ignore */ }
  }

  async function deleteTask(taskId) {
    if (!activeBoard) return;
    try {
      await deleteBoardTask(activeBoard.name, taskId);
      selectedTask = null;
      await selectBoard(activeBoard.name);
    } catch { /* ignore */ }
  }

  async function moveToBoard(taskId, toBoard) {
    if (!activeBoard || !toBoard) return;
    try {
      await moveTaskToBoard(activeBoard.name, taskId, toBoard);
      selectedTask = null;
      await selectBoard(activeBoard.name);
    } catch { /* ignore */ }
  }

  async function restoreTask(taskId) {
    if (!activeBoard) return;
    try {
      await updateBoardTask(activeBoard.name, taskId, { status: "backlog" });
      await selectBoard(activeBoard.name);
    } catch { /* ignore */ }
  }

  function archivedTasks() {
    return (activeBoard?.tasks || []).filter(t => t.status === "archived");
  }

  async function updateField(taskId, field, value) {
    if (!activeBoard) return;
    try {
      await updateBoardTask(activeBoard.name, taskId, { [field]: value });
      await selectBoard(activeBoard.name);
      if (selectedTask?.id === taskId) {
        const updated = activeBoard.tasks.find(t => t.id === taskId);
        if (updated) selectedTask = updated;
      }
    } catch { /* ignore */ }
  }

  function handleDragStart(task) {
    dragTask = task;
  }

  function handleDragOver(e, col) {
    e.preventDefault();
    dragOverCol = col;
  }

  function handleDragLeave() {
    dragOverCol = null;
  }

  async function handleDrop(e, col) {
    e.preventDefault();
    dragOverCol = null;
    if (dragTask && dragTask.status !== col) {
      await moveTask(dragTask.id, col);
    }
    dragTask = null;
  }

  function handleDragEnd() {
    dragTask = null;
    dragOverCol = null;
  }

  onMount(() => { load(); loadConfig(); });
</script>

<div class="board-page">
  {#if !loading && boards.length === 0}
    <div class="board-empty">
      <p style="font-size:1.1rem;font-weight:600;margin-bottom:.5rem">No boards yet</p>
      <p style="font-size:.85rem;color:var(--text-secondary);margin-bottom:1rem">
        Create a kanban board to track agentic tasks. Boards are stored as YAML files — human-readable and git-trackable.
      </p>
      <button class="btn btn-primary" onclick={() => { showCreateBoard = true; }}>Create your first board</button>
      <p style="font-size:.78rem;color:var(--text-muted);margin-top:.8rem">
        Or via CLI: <code>vibecockpit board create my-project --project ~/Projects/my-project</code>
      </p>
    </div>
  {:else}
    <!-- Board list + kanban layout -->
    <div class="board-layout">
      <aside class="board-sidebar">
        <div class="board-sidebar-header">
          <span class="board-sidebar-title">Boards</span>
          <button class="btn btn-sm" onclick={() => { showCreateBoard = true; }}>+</button>
        </div>
        {#each boards as b}
          {@const isActive = activeBoard?.name === b.name}
          {@const activeTasks = (b.tasks || []).filter(t => t.status !== "archived")}
          {@const workingCount = (b.tasks || []).filter(t => t.status === "in-progress").length}
          <div class="board-card-wrap">
            <button class="board-card" class:board-card-active={isActive} onclick={() => selectBoard(b.name)}>
              <div class="board-card-name">{b.name}</div>
              <div class="board-card-project" title={b.project}>{b.project}</div>
              <div class="board-card-stats">
                <span>{activeTasks.length} tasks</span>
                {#if workingCount > 0}
                  <span class="board-card-active-dot">&#9679; {workingCount} active</span>
                {/if}
              </div>
            </button>
            <button class="board-card-delete" title="Delete board" onclick={() => removeBoard(b.name)}>&#10005;</button>
          </div>
        {/each}
      </aside>

      <div class="board-main">
        {#if activeBoard}
          <div class="board-header">
            <div class="board-header-left">
              <h2>{activeBoard.name}</h2>
              <span class="board-meta" title={activeBoard.filePath}>{activeBoard.project}</span>
              {#if totalCost() > 0}
                <span class="board-cost">${totalCost().toFixed(2)}</span>
              {/if}
            </div>
            <div class="board-header-right">
              <div class="board-filters">
                <button class="filter-btn" class:filter-active={!filterPriority} onclick={() => { filterPriority = ""; }}>All</button>
                <button class="filter-btn filter-high" class:filter-active={filterPriority === "high"} onclick={() => { filterPriority = filterPriority === "high" ? "" : "high"; }}>High</button>
                <button class="filter-btn filter-medium" class:filter-active={filterPriority === "medium"} onclick={() => { filterPriority = filterPriority === "medium" ? "" : "medium"; }}>Medium</button>
                <button class="filter-btn filter-low" class:filter-active={filterPriority === "low"} onclick={() => { filterPriority = filterPriority === "low" ? "" : "low"; }}>Low</button>
              </div>
              <button class="btn btn-sm" onclick={() => { showAddTask = true; }}>+ Task</button>
              <button class="btn btn-sm" onclick={load}>Refresh</button>
            </div>
          </div>
    <div class="kanban">
      {#each columns() as col}
        {@const tasks = tasksByColumn(col)}
        <div
          class="kanban-col"
          class:kanban-col-dragover={dragOverCol === col}
          ondragover={(e) => handleDragOver(e, col)}
          ondragleave={handleDragLeave}
          ondrop={(e) => handleDrop(e, col)}
          role="list"
        >
          <div class="kanban-col-header">
            <span class="kanban-col-title">{columnLabels[col] || col}</span>
            <span class="kanban-col-count">{tasks.length}</span>
          </div>
          <div class="kanban-col-body">
            {#each tasks as task (task.id)}
              <div
                class="kanban-card"
                class:kanban-card-dragging={dragTask?.id === task.id}
                draggable="true"
                ondragstart={() => handleDragStart(task)}
                ondragend={handleDragEnd}
                role="listitem"
              >
                <button class="kanban-card-inner" onclick={() => { selectedTask = selectedTask?.id === task.id ? null : task; }}>
                  <div class="kanban-card-top">
                    {#if task.priority}
                      <span class="kanban-priority" style="color:{priorityColors[task.priority] || 'var(--text-muted)'}">{task.priority}</span>
                    {/if}
                    {#if task.cost}
                      <span class="kanban-cost">{task.cost}</span>
                    {/if}
                  </div>
                  <div class="kanban-card-title">{task.title}</div>
                  {#if task.createdBy}
                    <span class="kanban-origin" class:kanban-origin-agent={task.createdBy !== "human"}>{task.createdBy === "human" ? "manual" : task.createdBy}</span>
                  {/if}
                  {#if task.tool || task.claimedBy || task.model}
                    <div class="kanban-card-meta">
                      {#if task.claimedBy}
                        <span class="kanban-claimed">
                          <span class="kanban-dot" style="background:{providerColors[task.claimedBy] || '#888'}"></span>
                          {task.claimedBy}
                        </span>
                      {/if}
                      {#if task.tool}
                        <span class="kanban-tool">
                          <span class="kanban-dot" style="background:{providerColors[task.tool] || '#888'}"></span>
                          {task.tool}
                        </span>
                      {/if}
                      {#if task.model}
                        <span class="kanban-model">{task.model}</span>
                      {/if}
                    </div>
                  {/if}
                  {#if task.completed}
                    <div class="kanban-card-time">{relativeTime(task.completed)}</div>
                  {:else if task.started}
                    <div class="kanban-card-time">started {relativeTime(task.started)}</div>
                  {/if}
                </button>
              </div>
            {/each}
          </div>
        </div>
      {/each}
    </div>

    {#if archivedTasks().length > 0}
      <div class="archived-section">
        <button class="archived-toggle" onclick={() => { showArchived = !showArchived; }}>
          <span class="archived-chevron" class:archived-open={showArchived}>&#9660;</span>
          Archived ({archivedTasks().length})
        </button>
        {#if showArchived}
          <div class="archived-list">
            {#each archivedTasks() as task}
              <div class="archived-row">
                <span class="archived-title">{task.title}</span>
                <span class="archived-id">{task.id}</span>
                <button class="btn btn-sm" onclick={() => restoreTask(task.id)}>Restore</button>
                <button class="btn btn-sm btn-danger" onclick={() => deleteTask(task.id)}>Delete</button>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    {/if}

        {/if}
      </div>
    </div>
  {/if}
</div>

<!-- Task Detail Modal -->
{#if selectedTask}
<div class="board-overlay" onclick={() => { selectedTask = null; }} role="dialog" aria-modal="true">
  <div class="board-modal task-modal" onclick={(e) => e.stopPropagation()}>
    <div class="task-modal-header">
      <div style="display:flex;align-items:center;gap:.5rem">
        <code class="task-modal-id">{selectedTask.id}</code>
        {#if selectedTask.createdBy}
          <span class="kanban-origin" class:kanban-origin-agent={selectedTask.createdBy !== "human"}>{selectedTask.createdBy === "human" ? "manual" : selectedTask.createdBy}</span>
        {/if}
      </div>
      <button class="btn btn-sm" onclick={() => { selectedTask = null; }}>&#10005;</button>
    </div>
    <div class="field">
      <label for="edit-title">Title</label>
      <input id="edit-title" type="text" value={selectedTask.title}
        onchange={(e) => updateField(selectedTask.id, "title", e.target.value)} />
    </div>
    <div style="display:flex;gap:.6rem">
      <div class="field" style="flex:1">
        <label for="edit-status">Status</label>
        <select id="edit-status" value={selectedTask.status} onchange={(e) => updateField(selectedTask.id, "status", e.target.value)}>
          {#each columns() as col}
            <option value={col}>{columnLabels[col] || col}</option>
          {/each}
        </select>
      </div>
      <div class="field" style="flex:1">
        <label for="edit-priority">Priority</label>
        <select id="edit-priority" value={selectedTask.priority || ""} onchange={(e) => updateField(selectedTask.id, "priority", e.target.value)}>
          <option value="">None</option>
          <option value="high">High</option>
          <option value="medium">Medium</option>
          <option value="low">Low</option>
        </select>
      </div>
    </div>
    <div style="display:flex;gap:.6rem">
      <div class="field" style="flex:1">
        <label for="edit-tool">Tool</label>
        <select id="edit-tool" value={selectedTask.tool || ""} onchange={(e) => updateField(selectedTask.id, "tool", e.target.value)}>
          <option value="">Board default</option>
          {#each availableProviders as p}
            <option value={p}>{p}</option>
          {/each}
        </select>
      </div>
      <div class="field" style="flex:1">
        <label for="edit-model">Model</label>
        <select id="edit-model" value={selectedTask.model || ""} onchange={(e) => updateField(selectedTask.id, "model", e.target.value)}>
          <option value="">Board default</option>
          {#each modelsForTool(selectedTask.tool || activeBoard?.defaults?.tool) as m}
            <option value={m}>{m}</option>
          {/each}
        </select>
      </div>
    </div>
    <div class="field">
      <label for="edit-desc">Description</label>
      <textarea id="edit-desc" rows="4" value={selectedTask.description || ""}
        onchange={(e) => updateField(selectedTask.id, "description", e.target.value)}
        placeholder="Task details..."></textarea>
    </div>
    {#if selectedTask.acceptance?.length}
      <div class="field">
        <label>Acceptance criteria</label>
        <ul class="task-detail-accept">
          {#each selectedTask.acceptance as a}
            <li>{a}</li>
          {/each}
        </ul>
      </div>
    {/if}
    <div class="task-modal-meta">
      <div class="task-meta-timestamps">
        {#if selectedTask.createdAt}
          <span title={selectedTask.createdAt}>created {relativeTime(selectedTask.createdAt)}</span>
        {/if}
        {#if selectedTask.updatedAt && selectedTask.updatedAt !== selectedTask.createdAt}
          <span title={selectedTask.updatedAt}>updated {relativeTime(selectedTask.updatedAt)}</span>
        {/if}
        {#if selectedTask.started}
          <span title={selectedTask.started}>started {relativeTime(selectedTask.started)}</span>
        {/if}
        {#if selectedTask.completed}
          <span title={selectedTask.completed}>completed {relativeTime(selectedTask.completed)}</span>
        {/if}
      </div>
      {#if selectedTask.session}
        <div class="task-meta-row"><span class="task-meta-label">Session</span> <code>{selectedTask.session}</code></div>
      {/if}
      {#if selectedTask.cost}
        <div class="task-meta-row"><span class="task-meta-label">Cost</span> <span>{selectedTask.cost}</span></div>
      {/if}
      {#if selectedTask.summary}
        <div class="task-meta-row" style="flex-direction:column;align-items:stretch">
          <span class="task-meta-label">Agent summary</span>
          <p class="task-detail-desc">{selectedTask.summary}</p>
        </div>
      {/if}
    </div>
    {#if selectedTask.history?.length}
    <div class="task-history">
      <span class="task-history-title">History</span>
      <div class="task-history-list">
        {#each selectedTask.history as h}
          <div class="task-history-row">
            <span class="task-history-time" title={h.timestamp}>{relativeTime(h.timestamp)}</span>
            <span class="task-history-action">{h.action}</span>
            <span class="task-history-detail">{h.detail}</span>
            {#if h.by}
              <span class="kanban-origin" class:kanban-origin-agent={h.by !== "human"}>{h.by === "human" ? "manual" : h.by}</span>
            {/if}
          </div>
        {/each}
      </div>
    </div>
    {/if}
    <div class="task-modal-actions">
      {#if boards.length > 1}
        <select class="task-move-select" onchange={(e) => { if (e.target.value) moveToBoard(selectedTask.id, e.target.value); e.target.value = ""; }}>
          <option value="">Move to board...</option>
          {#each boards.filter(b => b.name !== activeBoard?.name) as b}
            <option value={b.name}>{b.name}</option>
          {/each}
        </select>
      {/if}
      <span style="flex:1"></span>
      {#if selectedTask.status === "archived"}
        <button class="btn btn-sm" onclick={() => restoreTask(selectedTask.id)}>Restore to backlog</button>
        <button class="btn btn-sm btn-danger" onclick={() => deleteTask(selectedTask.id)}>Delete permanently</button>
      {:else}
        <button class="btn btn-sm" onclick={() => archiveTask(selectedTask.id)}>Archive</button>
      {/if}
    </div>
  </div>
</div>
{/if}

<!-- Create Board Modal -->
{#if showCreateBoard}
<div class="board-overlay" onclick={() => { showCreateBoard = false; }} role="dialog" aria-modal="true">
  <div class="board-modal" onclick={(e) => e.stopPropagation()}>
    <h3>Create Board</h3>
    <div class="field">
      <label for="board-name">Board name</label>
      <input id="board-name" type="text" bind:value={newBoardName} placeholder="my-project" />
    </div>
    <div class="field">
      <label for="board-project">Project</label>
      {#if customProject || knownProjects.length === 0}
        <input id="board-project" type="text" bind:value={newBoardProject} placeholder="~/Projects/my-project" />
        {#if knownProjects.length > 0}
          <button class="link-btn" style="margin-top:.3rem;font-size:.75rem" onclick={() => { customProject = false; }}>Pick from existing projects</button>
        {/if}
      {:else}
        <select id="board-project" value={newBoardProject} onchange={(e) => {
          if (e.target.value === "__custom__") { customProject = true; newBoardProject = ""; }
          else {
            newBoardProject = e.target.value;
            if (!newBoardName.trim()) newBoardName = knownProjects.find(p => p.path === e.target.value)?.name || "";
          }
        }}>
          <option value="">Select a project...</option>
          {#each knownProjects as p}
            <option value={p.path}>{p.name} — {p.path}</option>
          {/each}
          <option value="__custom__">Enter custom path...</option>
        </select>
      {/if}
    </div>
    <div class="board-modal-actions">
      <button class="btn" onclick={() => { showCreateBoard = false; customProject = false; }}>Cancel</button>
      <button class="btn btn-primary" onclick={doCreateBoard} disabled={!newBoardName.trim()}>Create</button>
    </div>
  </div>
</div>
{/if}

<!-- Add Task Modal -->
{#if showAddTask}
<div class="board-overlay" onclick={() => { showAddTask = false; }} role="dialog" aria-modal="true">
  <div class="board-modal" onclick={(e) => e.stopPropagation()}>
    <h3>Add Task to {activeBoard?.name}</h3>
    <div class="field">
      <label for="task-title">Title</label>
      <input id="task-title" type="text" bind:value={newTaskTitle} placeholder="What needs to be done?" />
    </div>
    <div class="field">
      <label for="task-priority">Priority</label>
      <select id="task-priority" bind:value={newTaskPriority}>
        <option value="high">High</option>
        <option value="medium">Medium</option>
        <option value="low">Low</option>
      </select>
    </div>
    <div style="display:flex;gap:.6rem">
      <div class="field" style="flex:1">
        <label for="task-tool">Tool</label>
        <select id="task-tool" bind:value={newTaskTool}>
          <option value="">Board default</option>
          {#each availableProviders as p}
            <option value={p}>{p}</option>
          {/each}
        </select>
      </div>
      <div class="field" style="flex:1">
        <label for="task-model">Model</label>
        <select id="task-model" bind:value={newTaskModel}>
          <option value="">Board default</option>
          {#each modelsForTool(newTaskTool || activeBoard?.defaults?.tool) as m}
            <option value={m}>{m}</option>
          {/each}
        </select>
      </div>
    </div>
    <div class="field">
      <label for="task-desc">Description (optional)</label>
      <textarea id="task-desc" bind:value={newTaskDescription} placeholder="Details, context, constraints..." rows="3"></textarea>
    </div>
    <div class="board-modal-actions">
      <button class="btn" onclick={() => { showAddTask = false; }}>Cancel</button>
      <button class="btn btn-primary" onclick={doAddTask} disabled={!newTaskTitle.trim()}>Add Task</button>
    </div>
  </div>
</div>
{/if}

<style>
  .board-page { max-width: 1400px; margin: 1.5rem auto; padding: 0 1rem; }
  .board-empty { text-align: center; padding: 3rem 1rem; background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius); }

  /* Layout: sidebar + main */
  .board-layout { display: flex; gap: 1rem; min-height: 500px; }
  .board-sidebar { width: 200px; flex-shrink: 0; }
  .board-sidebar-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: .5rem; }
  .board-sidebar-title { font-size: .78rem; font-weight: 600; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .5px; }
  .board-card-wrap { position: relative; margin-bottom: .3rem; }
  .board-card { display: block; width: 100%; text-align: left; padding: .5rem .6rem; background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius-sm); cursor: pointer; font-family: inherit; color: var(--text); transition: border-color .15s, background .15s; }
  .board-card:hover { border-color: var(--primary); }
  .board-card-active { border-color: var(--primary); background: var(--primary-glow, rgba(99,102,241,.06)); }
  .board-card-name { font-size: .85rem; font-weight: 600; margin-bottom: .15rem; }
  .board-card-project { font-size: .7rem; color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; margin-bottom: .2rem; }
  .board-card-stats { font-size: .7rem; color: var(--text-secondary); display: flex; gap: .5rem; }
  .board-card-active-dot { color: var(--success); }
  .board-card-delete { position: absolute; top: .3rem; right: .3rem; background: none; border: none; cursor: pointer; color: var(--text-muted); font-size: .7rem; padding: .1rem .3rem; border-radius: 3px; opacity: 0; transition: opacity .15s, color .15s; }
  .board-card-wrap:hover .board-card-delete { opacity: 1; }
  .board-card-delete:hover { color: var(--danger); background: var(--danger-dim, rgba(239,68,68,.1)); }
  .board-main { flex: 1; min-width: 0; }

  /* Header within board main */
  .board-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 1rem; flex-wrap: wrap; gap: .5rem; }
  .board-header-left { display: flex; align-items: center; gap: .8rem; flex-wrap: wrap; }
  .board-header-left h2 { font-size: 1.2rem; margin: 0; }
  .board-header-right { display: flex; gap: .4rem; align-items: center; }
  .board-filters { display: flex; border: 1px solid var(--border); border-radius: var(--radius-sm); overflow: hidden; }
  .filter-btn { padding: .25rem .5rem; font-size: .72rem; font-weight: 500; background: none; border: none; border-right: 1px solid var(--border); cursor: pointer; font-family: inherit; color: var(--text-secondary); transition: background .15s, color .15s; }
  .filter-btn:last-child { border-right: none; }
  .filter-btn:hover { background: var(--surface-hover, var(--surface)); }
  .filter-active { background: var(--surface); color: var(--text); font-weight: 600; }
  .filter-high.filter-active { color: var(--danger); }
  .filter-medium.filter-active { color: var(--warning, #f59e0b); }
  .filter-low.filter-active { color: var(--text-muted); }
  .board-meta { font-size: .78rem; color: var(--text-secondary); }
  .board-cost { font-size: .78rem; font-weight: 600; color: var(--warning, #f59e0b); }

  /* Kanban */
  .kanban { display: flex; gap: .5rem; min-height: 300px; }
  .kanban-col { flex: 1; min-width: 0; background: var(--bg); border: 1px solid var(--border); border-radius: var(--radius); display: flex; flex-direction: column; transition: border-color .15s; }
  .kanban-col-dragover { border-color: var(--primary); background: var(--primary-glow, rgba(99,102,241,.05)); }
  .kanban-col-header { display: flex; align-items: center; justify-content: space-between; padding: .6rem .7rem; border-bottom: 1px solid var(--border); }
  .kanban-col-title { font-size: .78rem; font-weight: 600; text-transform: uppercase; letter-spacing: .5px; color: var(--text-secondary); }
  .kanban-col-count { font-size: .7rem; font-weight: 600; color: var(--text-muted); background: var(--surface); border: 1px solid var(--border); border-radius: 8px; padding: .05rem .35rem; }
  .kanban-col-body { flex: 1; padding: .4rem; display: flex; flex-direction: column; gap: .4rem; min-height: 60px; }

  /* Cards */
  .kanban-card { cursor: grab; transition: opacity .15s, transform .15s; }
  .kanban-card:active { cursor: grabbing; }
  .kanban-card-dragging { opacity: .4; transform: scale(.95); }
  .kanban-card-inner { display: block; width: 100%; text-align: left; padding: .5rem .6rem; background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius-sm); cursor: pointer; font-family: inherit; color: var(--text); transition: border-color .15s, box-shadow .15s; }
  .kanban-card-inner:hover { border-color: var(--primary); box-shadow: 0 2px 8px rgba(0,0,0,.06); }
  .kanban-card-top { display: flex; justify-content: space-between; align-items: center; margin-bottom: .2rem; }
  .kanban-card-title { font-size: .82rem; font-weight: 500; line-height: 1.3; }
  .kanban-priority { font-size: .65rem; font-weight: 600; text-transform: uppercase; letter-spacing: .5px; }
  .kanban-cost { font-size: .7rem; font-weight: 600; color: var(--warning, #f59e0b); }
  .kanban-card-meta { display: flex; gap: .4rem; align-items: center; margin-top: .3rem; font-size: .7rem; color: var(--text-secondary); }
  .kanban-claimed { display: flex; align-items: center; gap: .25rem; }
  .kanban-dot { width: 6px; height: 6px; border-radius: 50%; }
  .kanban-tool { display: flex; align-items: center; gap: .25rem; }
  .kanban-model { font-size: .65rem; color: var(--text-muted); background: var(--bg); padding: .05rem .3rem; border-radius: 3px; border: 1px solid var(--border); }
  .kanban-origin { font-size: .6rem; font-weight: 500; color: var(--text-muted); background: var(--bg); border: 1px solid var(--border); border-radius: 6px; padding: .05rem .3rem; text-transform: uppercase; letter-spacing: .3px; }
  .kanban-origin-agent { color: var(--primary); border-color: var(--primary); background: var(--primary-glow, rgba(99,102,241,.08)); }
  .kanban-card-time { font-size: .68rem; color: var(--text-muted); margin-top: .2rem; }

  /* Task detail modal */
  .task-modal { max-width: 520px; }
  .task-modal-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: .8rem; }
  .task-modal-id { font-size: .75rem; color: var(--text-muted); }
  .task-detail-desc { font-size: .82rem; color: var(--text-secondary); line-height: 1.5; margin: .3rem 0 0; white-space: pre-wrap; }
  .task-detail-accept { font-size: .82rem; color: var(--text-secondary); margin: .3rem 0 0; padding-left: 1.2rem; }
  .task-detail-accept li { margin-bottom: .2rem; }
  .task-modal-meta { margin-top: .6rem; padding-top: .6rem; border-top: 1px solid var(--border); }
  .task-meta-row { display: flex; gap: .6rem; align-items: center; margin-bottom: .3rem; font-size: .8rem; }
  .task-meta-label { font-size: .7rem; font-weight: 600; color: var(--text-muted); text-transform: uppercase; letter-spacing: .5px; min-width: 5.5rem; flex-shrink: 0; }
  .task-meta-timestamps { display: flex; flex-wrap: wrap; gap: .3rem .8rem; font-size: .72rem; color: var(--text-muted); margin-bottom: .4rem; }
  .task-history { margin-top: .6rem; padding-top: .6rem; border-top: 1px solid var(--border); }
  .task-history-title { font-size: .72rem; font-weight: 600; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .5px; }
  .task-history-list { margin-top: .3rem; }
  .task-history-row { display: flex; align-items: center; gap: .5rem; font-size: .75rem; padding: .2rem 0; border-bottom: 1px solid var(--border); }
  .task-history-row:last-child { border-bottom: none; }
  .task-history-time { color: var(--text-muted); font-size: .68rem; min-width: 5rem; flex-shrink: 0; }
  .task-history-action { font-weight: 500; min-width: 3.5rem; flex-shrink: 0; }
  .task-history-detail { color: var(--text-secondary); flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  /* Modals */
  .board-overlay { position: fixed; inset: 0; z-index: 100; background: rgba(0,0,0,.4); display: flex; align-items: center; justify-content: center; }
  .board-modal { background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius); padding: 1.5rem; width: 90%; max-width: 420px; }
  .board-modal h3 { font-size: 1rem; margin: 0 0 1rem; }
  .board-modal .field { margin-bottom: .8rem; }
  .board-modal .field label { display: block; font-size: .78rem; font-weight: 500; margin-bottom: .3rem; color: var(--text-secondary); }
  .board-modal .field input,
  .board-modal .field select,
  .board-modal .field textarea { width: 100%; font-size: .85rem; padding: .45rem .6rem; border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--bg); color: var(--text); font-family: inherit; box-sizing: border-box; }
  .board-modal .field textarea { resize: vertical; }
  .board-modal-actions { display: flex; gap: .5rem; justify-content: flex-end; margin-top: 1rem; }

  /* Task modal actions */
  .task-modal-actions { display: flex; gap: .4rem; align-items: center; margin-top: .8rem; padding-top: .6rem; border-top: 1px solid var(--border); }
  .task-move-select { font-size: .78rem; padding: .2rem .4rem; border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--bg); color: var(--text); font-family: inherit; }
  .btn-danger { color: var(--danger) !important; }
  .btn-danger:hover { background: var(--danger-dim, rgba(239,68,68,.1)); }

  /* Archived section */
  .archived-section { margin-top: 1rem; }
  .archived-toggle { display: flex; align-items: center; gap: .4rem; background: none; border: none; cursor: pointer; font-family: inherit; font-size: .8rem; font-weight: 500; color: var(--text-secondary); padding: .3rem 0; }
  .archived-toggle:hover { color: var(--text); }
  .archived-chevron { font-size: .6rem; transition: transform .15s; transform: rotate(-90deg); }
  .archived-chevron.archived-open { transform: rotate(0deg); }
  .archived-list { margin-top: .4rem; border: 1px solid var(--border); border-radius: var(--radius-sm); overflow: hidden; }
  .archived-row { display: flex; align-items: center; gap: .6rem; padding: .4rem .6rem; border-bottom: 1px solid var(--border); font-size: .8rem; }
  .archived-row:last-child { border-bottom: none; }
  .archived-title { flex: 1; font-weight: 500; }
  .archived-id { font-size: .72rem; color: var(--text-muted); }

  .link-btn { background: none; border: none; color: var(--primary); cursor: pointer; font-family: inherit; font-size: inherit; padding: 0; text-decoration: underline; }
  .link-btn:hover { color: var(--primary-hover, var(--primary)); }

  @media (max-width: 900px) {
    .board-layout { flex-direction: column; }
    .board-sidebar { width: 100%; display: flex; gap: .3rem; flex-wrap: wrap; }
    .board-sidebar-header { width: 100%; }
    .board-card-wrap { flex: 1; min-width: 140px; }
    .board-card-delete { opacity: 1; }
  }
  @media (max-width: 700px) {
    .kanban { flex-direction: column; }
    .kanban-col { min-width: 0; }
    .kanban-col-header { padding: .4rem .5rem; }
  }
</style>
