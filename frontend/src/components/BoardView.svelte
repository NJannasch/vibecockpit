<script>
  import { onMount } from "svelte";
  import { fetchBoards, fetchBoard, createBoard, addBoardTask, updateBoardTask } from "../lib/api.js";
  import { relativeTime, providerColors } from "../lib/utils.js";

  let boards = $state([]);
  let activeBoard = $state(null);
  let loading = $state(true);
  let showCreateBoard = $state(false);
  let showAddTask = $state(false);
  let selectedTask = $state(null);

  let newBoardName = $state("");
  let newBoardProject = $state("");
  let newTaskTitle = $state("");
  let newTaskPriority = $state("medium");
  let newTaskDescription = $state("");

  let dragTask = $state(null);
  let dragOverCol = $state(null);

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
    return (activeBoard?.tasks || []).filter(t => t.status === col);
  }

  function totalCost() {
    return (activeBoard?.tasks || [])
      .filter(t => t.cost)
      .reduce((sum, t) => sum + parseFloat(t.cost.replace("$", "") || 0), 0);
  }

  async function load() {
    loading = true;
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
      newBoardName = "";
      newBoardProject = "";
      await load();
      await selectBoard(newBoardName.trim() || boards[boards.length - 1]?.name);
    } catch { /* ignore */ }
  }

  async function doAddTask() {
    if (!newTaskTitle.trim() || !activeBoard) return;
    try {
      await addBoardTask(activeBoard.name, newTaskTitle.trim(), newTaskPriority, newTaskDescription.trim());
      showAddTask = false;
      newTaskTitle = "";
      newTaskPriority = "medium";
      newTaskDescription = "";
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

  onMount(load);
</script>

<div class="board-page">
  <div class="board-header">
    <div class="board-header-left">
      <h2>Boards</h2>
      {#if boards.length > 1}
        <select class="board-select" value={activeBoard?.name || ""} onchange={(e) => selectBoard(e.target.value)}>
          {#each boards as b}
            <option value={b.name}>{b.name}</option>
          {/each}
        </select>
      {/if}
      {#if activeBoard}
        <span class="board-meta">{activeBoard.project}</span>
        {#if totalCost() > 0}
          <span class="board-cost">${totalCost().toFixed(2)}</span>
        {/if}
      {/if}
    </div>
    <div class="board-header-right">
      <button class="btn btn-sm" onclick={() => { showAddTask = true; }}>+ Task</button>
      <button class="btn btn-sm" onclick={() => { showCreateBoard = true; }}>+ Board</button>
      <button class="btn btn-sm" onclick={load}>Refresh</button>
    </div>
  </div>

  {#if loading}
    <div class="board-empty">Loading boards...</div>
  {:else if boards.length === 0}
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
  {:else if activeBoard}
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
                  {#if task.tool || task.claimedBy}
                    <div class="kanban-card-meta">
                      {#if task.claimedBy}
                        <span class="kanban-claimed">
                          <span class="kanban-dot" style="background:{providerColors[task.claimedBy] || '#888'}"></span>
                          {task.claimedBy}
                        </span>
                      {/if}
                      {#if task.tool}
                        <span class="kanban-tool">{task.tool}</span>
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

    {#if selectedTask}
      <div class="task-detail">
        <div class="task-detail-header">
          <h3>{selectedTask.title}</h3>
          <button class="btn btn-sm" onclick={() => { selectedTask = null; }}>Close</button>
        </div>
        <div class="task-detail-body">
          <div class="task-detail-row">
            <span class="task-detail-label">ID</span>
            <code>{selectedTask.id}</code>
          </div>
          <div class="task-detail-row">
            <span class="task-detail-label">Status</span>
            <select value={selectedTask.status} onchange={(e) => moveTask(selectedTask.id, e.target.value)}>
              {#each columns() as col}
                <option value={col}>{columnLabels[col] || col}</option>
              {/each}
            </select>
          </div>
          {#if selectedTask.priority}
            <div class="task-detail-row">
              <span class="task-detail-label">Priority</span>
              <span style="color:{priorityColors[selectedTask.priority]}">{selectedTask.priority}</span>
            </div>
          {/if}
          {#if selectedTask.description}
            <div class="task-detail-row" style="flex-direction:column;align-items:stretch">
              <span class="task-detail-label">Description</span>
              <p class="task-detail-desc">{selectedTask.description}</p>
            </div>
          {/if}
          {#if selectedTask.acceptance?.length}
            <div class="task-detail-row" style="flex-direction:column;align-items:stretch">
              <span class="task-detail-label">Acceptance criteria</span>
              <ul class="task-detail-accept">
                {#each selectedTask.acceptance as a}
                  <li>{a}</li>
                {/each}
              </ul>
            </div>
          {/if}
          {#if selectedTask.tool}
            <div class="task-detail-row">
              <span class="task-detail-label">Tool</span>
              <span>{selectedTask.tool}</span>
            </div>
          {/if}
          {#if selectedTask.model}
            <div class="task-detail-row">
              <span class="task-detail-label">Model</span>
              <span>{selectedTask.model}</span>
            </div>
          {/if}
          {#if selectedTask.session}
            <div class="task-detail-row">
              <span class="task-detail-label">Session</span>
              <code>{selectedTask.session}</code>
            </div>
          {/if}
          {#if selectedTask.cost}
            <div class="task-detail-row">
              <span class="task-detail-label">Cost</span>
              <span>{selectedTask.cost}</span>
            </div>
          {/if}
          {#if selectedTask.summary}
            <div class="task-detail-row" style="flex-direction:column;align-items:stretch">
              <span class="task-detail-label">Summary</span>
              <p class="task-detail-desc">{selectedTask.summary}</p>
            </div>
          {/if}
        </div>
      </div>
    {/if}
  {/if}
</div>

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
      <label for="board-project">Project directory</label>
      <input id="board-project" type="text" bind:value={newBoardProject} placeholder="~/Projects/my-project" />
    </div>
    <div class="board-modal-actions">
      <button class="btn" onclick={() => { showCreateBoard = false; }}>Cancel</button>
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
  .board-page { max-width: 1200px; margin: 1.5rem auto; padding: 0 1rem; }
  .board-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 1rem; flex-wrap: wrap; gap: .5rem; }
  .board-header-left { display: flex; align-items: center; gap: .8rem; flex-wrap: wrap; }
  .board-header-left h2 { font-size: 1.2rem; margin: 0; }
  .board-header-right { display: flex; gap: .4rem; }
  .board-select { font-size: .85rem; padding: .3rem .5rem; border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--surface); color: var(--text); font-family: inherit; }
  .board-meta { font-size: .78rem; color: var(--text-secondary); }
  .board-cost { font-size: .78rem; font-weight: 600; color: var(--warning, #f59e0b); }
  .board-empty { text-align: center; padding: 3rem 1rem; background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius); }

  /* Kanban */
  .kanban { display: flex; gap: .6rem; overflow-x: auto; padding-bottom: 1rem; min-height: 400px; }
  .kanban-col { flex: 1; min-width: 180px; max-width: 260px; background: var(--bg); border: 1px solid var(--border); border-radius: var(--radius); display: flex; flex-direction: column; transition: border-color .15s; }
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
  .kanban-tool { font-style: italic; }
  .kanban-card-time { font-size: .68rem; color: var(--text-muted); margin-top: .2rem; }

  /* Task detail */
  .task-detail { background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius); margin-top: 1rem; }
  .task-detail-header { display: flex; align-items: center; justify-content: space-between; padding: .8rem 1rem; border-bottom: 1px solid var(--border); }
  .task-detail-header h3 { font-size: .95rem; margin: 0; }
  .task-detail-body { padding: .8rem 1rem; }
  .task-detail-row { display: flex; gap: .8rem; align-items: center; margin-bottom: .5rem; font-size: .85rem; }
  .task-detail-label { font-size: .72rem; font-weight: 600; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .5px; min-width: 6rem; flex-shrink: 0; }
  .task-detail-desc { font-size: .82rem; color: var(--text-secondary); line-height: 1.5; margin: .3rem 0 0; white-space: pre-wrap; }
  .task-detail-accept { font-size: .82rem; color: var(--text-secondary); margin: .3rem 0 0; padding-left: 1.2rem; }
  .task-detail-accept li { margin-bottom: .2rem; }
  .task-detail-row select { font-size: .82rem; padding: .2rem .4rem; border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--bg); color: var(--text); font-family: inherit; }

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

  @media (max-width: 700px) {
    .kanban { flex-direction: column; }
    .kanban-col { max-width: none; min-width: 0; }
  }
</style>
