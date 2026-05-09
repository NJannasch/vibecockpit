<script>
  import { onMount } from "svelte";

  let chats = $state([]);
  let activeChat = $state(null);
  let sending = $state(false);
  let messageInput = $state("");
  let msgTool = $state("");
  let msgModel = $state("");
  let chatFiles = $state([]);
  let showFiles = $state(false);
  let viewingFile = $state(null);
  let showNewChat = $state(false);
  let newChatForm = $state({ name: "", tool: "claude", model: "", project: "", mcpServers: "vibecockpit" });
  let knownProjects = $state([]);
  let modelsByProvider = $state({});
  let allModels = $state([]);
  let messagesEnd = $state();

  async function loadSessionData() {
    try {
      const r = await fetch("/api/sessions");
      if (!r.ok) return;
      const sessions = await r.json();
      const seen = new Map();
      const provModels = {};
      const allM = new Set();
      for (const s of sessions) {
        if (s.projectPath && !seen.has(s.projectPath)) seen.set(s.projectPath, s.projectName || s.projectPath.split("/").pop());
        if (s.model && s.provider) {
          if (!provModels[s.provider]) provModels[s.provider] = new Set();
          provModels[s.provider].add(s.model);
          allM.add(s.model);
        }
      }
      knownProjects = [...seen.entries()].map(([path, name]) => ({ path, name })).sort((a, b) => a.name.localeCompare(b.name));
      modelsByProvider = Object.fromEntries(Object.entries(provModels).map(([k, v]) => [k, [...v].sort()]));
      allModels = [...allM].sort();
    } catch { /* ignore */ }
  }

  function modelsForTool(tool) {
    return modelsByProvider[tool] || allModels;
  }

  async function loadChats() {
    try {
      const r = await fetch("/api/chats");
      if (r.ok) chats = await r.json();
    } catch { /* ignore */ }
  }

  async function selectChat(id) {
    try {
      const r = await fetch(`/api/chats/${encodeURIComponent(id)}`);
      if (r.ok) activeChat = await r.json();
    } catch { /* ignore */ }
    viewingFile = null;
    loadFiles(id);
    scrollToBottom();
  }

  async function loadFiles(id) {
    try {
      const r = await fetch(`/api/chats/${encodeURIComponent(id)}/files`);
      if (r.ok) chatFiles = await r.json();
      else chatFiles = [];
    } catch { chatFiles = []; }
  }

  async function openFile(name) {
    if (!activeChat) return;
    try {
      const r = await fetch(`/api/chats/${encodeURIComponent(activeChat.id)}/files/${encodeURIComponent(name)}`);
      if (r.ok) viewingFile = await r.json();
    } catch { /* ignore */ }
  }

  function formatSize(bytes) {
    if (bytes < 1024) return bytes + " B";
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KB";
    return (bytes / (1024 * 1024)).toFixed(1) + " MB";
  }

  async function createChat() {
    if (!newChatForm.name.trim()) return;
    try {
      const body = {
        name: newChatForm.name,
        tool: newChatForm.tool,
        model: newChatForm.model,
        project: newChatForm.project,
        mcpServers: newChatForm.mcpServers.split(',').map(s => s.trim()).filter(Boolean),
      };
      const r = await fetch("/api/chats", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      if (r.ok) {
        const c = await r.json();
        showNewChat = false;
        newChatForm = { name: "", tool: "claude", model: "", project: "", mcpServers: "vibecockpit" };
        await loadChats();
        await selectChat(c.id);
      }
    } catch { /* ignore */ }
  }

  async function deleteChat(id) {
    try {
      await fetch(`/api/chats/${encodeURIComponent(id)}`, { method: "DELETE" });
      if (activeChat?.id === id) activeChat = null;
      await loadChats();
    } catch { /* ignore */ }
  }

  async function sendMessage() {
    if (!messageInput.trim() || !activeChat || sending) return;
    const content = messageInput.trim();
    const tool = msgTool || activeChat.tool;
    const model = msgModel || activeChat.model;
    messageInput = "";
    sending = true;

    activeChat.messages = [...activeChat.messages, { role: "user", content, timestamp: new Date().toISOString(), tool, model }];
    scrollToBottom();

    try {
      const body = { content };
      if (msgTool) body.tool = msgTool;
      if (msgModel) body.model = msgModel;
      const r = await fetch(`/api/chats/${encodeURIComponent(activeChat.id)}/message`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      if (r.ok) {
        const msg = await r.json();
        activeChat.messages = [...activeChat.messages, msg];
      } else {
        const err = await r.json();
        activeChat.messages = [...activeChat.messages, { role: "assistant", content: "Error: " + (err.error || "unknown"), timestamp: new Date().toISOString() }];
      }
    } catch (e) {
      activeChat.messages = [...activeChat.messages, { role: "assistant", content: "Error: " + e.message, timestamp: new Date().toISOString() }];
    }
    sending = false;
    scrollToBottom();
    if (activeChat) loadFiles(activeChat.id);
  }

  function scrollToBottom() {
    setTimeout(() => { messagesEnd?.scrollIntoView({ behavior: "smooth" }); }, 50);
  }

  function renderMarkdown(text) {
    let html = text
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;");
    html = html.replace(/```(\w*)\n([\s\S]*?)```/g, '<pre class="chat-code"><code>$2</code></pre>');
    html = html.replace(/`([^`]+)`/g, '<code class="chat-inline-code">$1</code>');
    html = html.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
    html = html.replace(/^### (.+)$/gm, '<strong class="chat-h3">$1</strong>');
    html = html.replace(/^## (.+)$/gm, '<strong class="chat-h2">$1</strong>');
    html = html.replace(/^# (.+)$/gm, '<strong class="chat-h1">$1</strong>');
    html = html.replace(/^- (.+)$/gm, '<span class="chat-li">• $1</span>');
    html = html.replace(/\n/g, '<br>');
    return html;
  }

  function handleKey(e) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  }

  onMount(() => {
    loadChats();
    loadSessionData();
  });
</script>

<div class="chat-page">
  <div class="chat-sidebar">
    <button class="chat-new-btn" onclick={() => { showNewChat = true; }}>+ New Chat</button>

    {#if chats.length === 0 && !showNewChat}
      <div class="chat-sidebar-empty">No chats yet</div>
    {/if}

    {#each chats as c (c.id)}
      <button
        class="chat-item"
        class:chat-item-active={activeChat?.id === c.id}
        onclick={() => selectChat(c.id)}
      >
        <div class="chat-item-name">{c.name}</div>
        <div class="chat-item-meta">{c.tool} · {c.messages?.length || 0} msgs</div>
      </button>
    {/each}
  </div>

  <div class="chat-main">
    {#if showNewChat}
      <div class="chat-new-form">
        <h3>New Chat</h3>
        <div class="chat-form-field">
          <label for="chat-name">Name</label>
          <input id="chat-name" type="text" bind:value={newChatForm.name} placeholder="e.g. Project planning" />
        </div>
        <div class="chat-form-row">
          <div class="chat-form-field">
            <label for="chat-tool">Tool</label>
            <select id="chat-tool" bind:value={newChatForm.tool} onchange={() => { newChatForm.model = ''; }}>
              <option value="claude">Claude</option>
              <option value="codex">Codex</option>
              <option value="gemini">Gemini</option>
              <option value="opencode">OpenCode</option>
            </select>
          </div>
          <div class="chat-form-field">
            <label for="chat-model">Model</label>
            <select id="chat-model" bind:value={newChatForm.model}>
              <option value="">Default</option>
              {#each modelsForTool(newChatForm.tool) as m (m)}
                <option value={m}>{m}</option>
              {/each}
            </select>
          </div>
        </div>
        <div class="chat-form-field">
          <label for="chat-project">Project (optional)</label>
          <select id="chat-project" bind:value={newChatForm.project}>
            <option value="">None — general chat</option>
            {#each knownProjects as p (p.path)}
              <option value={p.path}>{p.name} — {p.path}</option>
            {/each}
          </select>
        </div>
        <div class="chat-form-field">
          <label for="chat-mcp">MCP Servers</label>
          <input id="chat-mcp" type="text" bind:value={newChatForm.mcpServers} placeholder="vibecockpit, postgres" />
          <span class="chat-form-hint">Comma-separated. vibecockpit is always included.</span>
        </div>
        <div class="chat-form-actions">
          <button class="btn-secondary" onclick={() => { showNewChat = false; }}>Cancel</button>
          <button class="btn-primary" onclick={createChat} disabled={!newChatForm.name.trim()}>Create</button>
        </div>
      </div>
    {:else if activeChat}
      <div class="chat-header">
        <div>
          <span class="chat-header-name">{activeChat.name}</span>
          <span class="chat-header-meta">{activeChat.tool}{activeChat.model ? " / " + activeChat.model : ""}{activeChat.project ? " · " + activeChat.project.split('/').pop() : ""}</span>
        </div>
        <div class="chat-header-actions">
          {#if chatFiles.length > 0}
            <button class="chat-files-toggle" onclick={() => { showFiles = !showFiles; }} title="Files ({chatFiles.length})">
              {chatFiles.length} file{chatFiles.length !== 1 ? 's' : ''}
            </button>
          {/if}
          <button class="chat-delete-btn" onclick={() => deleteChat(activeChat.id)} title="Delete chat">&#128465;</button>
        </div>
      </div>

      {#if showFiles && chatFiles.length > 0}
        <div class="chat-files-panel">
          {#if viewingFile}
            <div class="chat-file-viewer">
              <div class="chat-file-viewer-header">
                <span class="chat-file-viewer-name">{viewingFile.name}</span>
                <button class="chat-file-close" onclick={() => { viewingFile = null; }}>Close</button>
              </div>
              <pre class="chat-file-content">{viewingFile.content}</pre>
            </div>
          {:else}
            <div class="chat-file-list">
              {#each chatFiles as f (f.name)}
                <button class="chat-file-item" onclick={() => openFile(f.name)}>
                  <span class="chat-file-icon">{f.isDir ? '📁' : '📄'}</span>
                  <span class="chat-file-name">{f.name}</span>
                  <span class="chat-file-size">{formatSize(f.size)}</span>
                </button>
              {/each}
            </div>
          {/if}
        </div>
      {/if}

      <div class="chat-messages">
        {#if activeChat.messages.length === 0}
          <div class="chat-empty-msg">Start the conversation. The agent has access to VibeCockpit MCP — ask it to create tasks, schedule jobs, check costs, etc.</div>
        {/if}
        {#each activeChat.messages as msg, i (i)}
          <div class="chat-msg chat-msg-{msg.role}">
            <div class="chat-msg-role">
              {msg.role === 'user' ? 'You' : (msg.tool || activeChat.tool)}
              {#if msg.model && msg.role === 'assistant'}
                <span class="chat-msg-model">{msg.model}</span>
              {/if}
            </div>
            {#if msg.role === "assistant"}
              <div class="chat-msg-content">{@html renderMarkdown(msg.content)}</div><!-- eslint-disable-line svelte/no-at-html-tags -->
            {:else}
              <div class="chat-msg-content">{msg.content}</div>
            {/if}
          </div>
        {/each}
        {#if sending}
          <div class="chat-msg chat-msg-assistant">
            <div class="chat-msg-role">{activeChat.tool}</div>
            <div class="chat-msg-content chat-typing">Thinking...</div>
          </div>
        {/if}
        <div bind:this={messagesEnd}></div>
      </div>

      <div class="chat-input-area">
        <div class="chat-input-row">
          <textarea
            class="chat-input"
            bind:value={messageInput}
            onkeydown={handleKey}
            placeholder="Type a message... (Enter to send, Shift+Enter for newline)"
            rows="2"
            disabled={sending}
          ></textarea>
          <button class="chat-send-btn" onclick={sendMessage} disabled={!messageInput.trim() || sending}>
            {sending ? '...' : 'Send'}
          </button>
        </div>
        <div class="chat-input-opts">
          <select class="chat-opt-select" bind:value={msgTool} title="Tool for this message">
            <option value="">{activeChat.tool}</option>
            <option value="claude">Claude</option>
            <option value="codex">Codex</option>
            <option value="gemini">Gemini</option>
            <option value="opencode">OpenCode</option>
          </select>
          <select class="chat-opt-select" bind:value={msgModel} title="Model for this message">
            <option value="">{activeChat.model || 'default'}</option>
            {#each modelsForTool(msgTool || activeChat.tool) as m (m)}
              <option value={m}>{m}</option>
            {/each}
          </select>
          <span class="chat-opt-hint">Override per message</span>
        </div>
      </div>
    {:else}
      <div class="chat-placeholder">
        <p>Select a chat or create a new one.</p>
        <p class="chat-placeholder-hint">Chats have VibeCockpit MCP — ask the agent to create tickets, schedule jobs, check session costs, or work on files in a project.</p>
      </div>
    {/if}
  </div>
</div>

<style>
  .chat-page { display: flex; height: calc(100vh - 110px); min-height: 0; overflow: hidden; }

  .chat-sidebar {
    width: 220px; flex-shrink: 0; border-right: 1px solid var(--border);
    padding: .5rem; overflow-y: auto; background: var(--bg);
  }
  .chat-new-btn {
    width: 100%; padding: .5rem; border: 1px dashed var(--border); border-radius: var(--radius-sm);
    background: none; color: var(--primary); font-family: inherit; font-size: .82rem;
    font-weight: 600; cursor: pointer; margin-bottom: .5rem;
  }
  .chat-new-btn:hover { background: var(--primary-glow, rgba(99,102,241,.06)); }
  .chat-sidebar-empty { text-align: center; padding: 1rem; font-size: .8rem; color: var(--text-muted); }

  .chat-item {
    width: 100%; padding: .5rem .6rem; border: 1px solid var(--border); border-radius: var(--radius-sm);
    background: var(--surface); cursor: pointer; font-family: inherit; color: var(--text);
    text-align: left; margin-bottom: .3rem; transition: border-color .15s;
  }
  .chat-item:hover { border-color: var(--primary); }
  .chat-item-active { border-color: var(--primary); background: var(--primary-glow, rgba(99,102,241,.06)); }
  .chat-item-name { font-size: .82rem; font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .chat-item-meta { font-size: .68rem; color: var(--text-muted); }

  .chat-main { flex: 1; display: flex; flex-direction: column; min-width: 0; min-height: 0; overflow: hidden; }

  .chat-header {
    display: flex; justify-content: space-between; align-items: center;
    padding: .6rem 1rem; border-bottom: 1px solid var(--border); background: var(--surface);
  }
  .chat-header-name { font-weight: 600; font-size: .9rem; }
  .chat-header-meta { font-size: .75rem; color: var(--text-muted); margin-left: .5rem; }
  .chat-header-actions { display: flex; align-items: center; gap: .4rem; }
  .chat-files-toggle {
    background: var(--bg); border: 1px solid var(--border); border-radius: var(--radius-sm);
    padding: .25rem .5rem; font-size: .72rem; cursor: pointer; color: var(--text); font-family: inherit;
  }
  .chat-files-toggle:hover { border-color: var(--primary); color: var(--primary); }
  .chat-delete-btn { background: none; border: none; cursor: pointer; font-size: 1rem; opacity: .5; }
  .chat-delete-btn:hover { opacity: 1; }

  .chat-files-panel {
    border-bottom: 1px solid var(--border); background: var(--bg); max-height: 250px; overflow-y: auto;
  }
  .chat-file-list { padding: .4rem .6rem; }
  .chat-file-item {
    display: flex; align-items: center; gap: .4rem; width: 100%; padding: .3rem .4rem;
    background: none; border: none; border-radius: var(--radius-sm); cursor: pointer;
    font-family: inherit; font-size: .8rem; color: var(--text); text-align: left;
  }
  .chat-file-item:hover { background: var(--surface); }
  .chat-file-icon { font-size: .75rem; }
  .chat-file-name { flex: 1; font-family: monospace; font-size: .78rem; }
  .chat-file-size { font-size: .68rem; color: var(--text-muted); }

  .chat-file-viewer { padding: .4rem .6rem; }
  .chat-file-viewer-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: .3rem; }
  .chat-file-viewer-name { font-family: monospace; font-size: .8rem; font-weight: 600; }
  .chat-file-close {
    background: none; border: 1px solid var(--border); border-radius: var(--radius-sm);
    padding: .15rem .4rem; font-size: .7rem; cursor: pointer; color: var(--text);
  }
  .chat-file-content {
    font-family: monospace; font-size: .75rem; background: var(--surface); border: 1px solid var(--border);
    border-radius: var(--radius-sm); padding: .5rem .6rem; margin: 0; max-height: 180px;
    overflow: auto; white-space: pre-wrap; word-break: break-all;
  }

  .chat-messages {
    flex: 1; overflow-y: auto; padding: 1rem; display: flex; flex-direction: column; gap: .6rem;
  }
  .chat-empty-msg {
    text-align: center; color: var(--text-muted); font-size: .85rem;
    padding: 2rem 1rem; max-width: 400px; margin: auto;
  }

  .chat-msg { max-width: 80%; padding: .6rem .8rem; border-radius: 10px; font-size: .85rem; line-height: 1.5; }
  .chat-msg-user { align-self: flex-end; background: var(--primary); color: white; border-bottom-right-radius: 2px; }
  .chat-msg-assistant { align-self: flex-start; background: var(--surface); border: 1px solid var(--border); border-bottom-left-radius: 2px; }
  .chat-msg-role { font-size: .65rem; font-weight: 600; opacity: .7; margin-bottom: .2rem; text-transform: uppercase; letter-spacing: .3px; }
  .chat-msg-content { white-space: pre-wrap; word-break: break-word; }
  .chat-msg-content :global(.chat-code) { background: var(--bg); padding: .4rem .6rem; border-radius: 4px; font-size: .78rem; overflow-x: auto; display: block; margin: .3rem 0; white-space: pre; }
  .chat-msg-content :global(.chat-inline-code) { background: var(--bg); padding: .1rem .3rem; border-radius: 3px; font-size: .8rem; }
  .chat-msg-content :global(.chat-h1) { font-size: 1rem; display: block; margin: .4rem 0 .2rem; }
  .chat-msg-content :global(.chat-h2) { font-size: .9rem; display: block; margin: .3rem 0 .2rem; }
  .chat-msg-content :global(.chat-h3) { font-size: .85rem; display: block; margin: .2rem 0; }
  .chat-msg-content :global(.chat-li) { display: block; padding-left: .5rem; }
  .chat-typing { color: var(--text-muted); font-style: italic; }

  .chat-msg-model { font-weight: 400; opacity: .6; margin-left: .3rem; }
  .chat-form-hint { font-size: .7rem; color: var(--text-muted); margin-top: .2rem; display: block; }

  .chat-input-area {
    padding: .6rem 1rem; border-top: 1px solid var(--border); background: var(--surface);
  }
  .chat-input-row { display: flex; gap: .4rem; }
  .chat-input {
    flex: 1; padding: .5rem .7rem; border: 1px solid var(--border); border-radius: var(--radius-sm);
    font-family: inherit; font-size: .85rem; resize: none; background: var(--bg); color: var(--text);
  }
  .chat-input:focus { outline: none; border-color: var(--primary); }
  .chat-send-btn {
    padding: .5rem 1rem; background: var(--primary); color: white; border: none;
    border-radius: var(--radius-sm); font-weight: 600; font-size: .82rem; cursor: pointer;
  }
  .chat-send-btn:disabled { opacity: .5; cursor: not-allowed; }

  .chat-input-opts {
    display: flex; gap: .4rem; align-items: center; margin-top: .3rem;
  }
  .chat-opt-select {
    padding: .2rem .4rem; border: 1px solid var(--border); border-radius: var(--radius-sm);
    font-size: .72rem; background: var(--bg); color: var(--text); cursor: pointer;
  }
  .chat-opt-hint { font-size: .65rem; color: var(--text-muted); margin-left: auto; }

  .chat-placeholder {
    flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center;
    color: var(--text-muted); text-align: center; padding: 2rem;
  }
  .chat-placeholder p { margin: .3rem 0; }
  .chat-placeholder-hint { font-size: .82rem; max-width: 400px; }

  .chat-new-form { padding: 1.5rem; max-width: 450px; margin: 2rem auto; }
  .chat-new-form h3 { margin: 0 0 1rem; font-size: 1.1rem; }
  .chat-form-field { margin-bottom: .8rem; }
  .chat-form-field label { display: block; font-size: .75rem; font-weight: 600; color: var(--text-muted); margin-bottom: .2rem; }
  .chat-form-field input, .chat-form-field select {
    width: 100%; padding: .5rem .6rem; border: 1px solid var(--border); border-radius: var(--radius-sm);
    font-size: .85rem; box-sizing: border-box; background: var(--surface); color: var(--text);
  }
  .chat-form-row { display: flex; gap: .6rem; }
  .chat-form-row .chat-form-field { flex: 1; }
  .chat-form-actions { display: flex; gap: .4rem; justify-content: flex-end; margin-top: 1rem; }

  .btn-primary { background: var(--primary); color: white; border: none; padding: .5rem 1rem; border-radius: var(--radius-sm); font-size: .82rem; cursor: pointer; font-weight: 600; }
  .btn-primary:disabled { opacity: .5; cursor: not-allowed; }
  .btn-secondary { background: var(--surface); color: var(--text); border: 1px solid var(--border); padding: .5rem 1rem; border-radius: var(--radius-sm); font-size: .82rem; cursor: pointer; }
</style>
