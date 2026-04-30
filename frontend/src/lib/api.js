const JSON_HEADERS = { "Content-Type": "application/json" };

/**
 * GET /api/sessions — Fetch all sessions.
 * @returns {Promise<Array>} Array of session objects.
 */
export async function fetchSessions(forceRefresh = false) {
  const url = forceRefresh ? "/api/sessions?refresh=true" : "/api/sessions";
  const res = await fetch(url);
  if (!res.ok) throw new Error(`Failed to fetch sessions: ${res.statusText}`);
  return res.json();
}

/**
 * GET /api/config — Fetch configuration.
 * @returns {Promise<Object>} Config object.
 */
export async function fetchConfig() {
  const res = await fetch("/api/config");
  if (!res.ok) throw new Error(`Failed to fetch config: ${res.statusText}`);
  return res.json();
}

/**
 * PUT /api/config — Save configuration.
 * @param {Object} config — Full config object to persist.
 * @returns {Promise<Object>} Response body.
 */
export async function saveConfig(config) {
  const res = await fetch("/api/config", {
    method: "PUT",
    headers: JSON_HEADERS,
    body: JSON.stringify(config),
  });
  if (!res.ok) throw new Error(`Failed to save config: ${res.statusText}`);
  return res.json();
}

/**
 * POST /api/launch — Launch / resume a session.
 * @param {string} sessionId
 * @param {string} provider
 * @param {string} [model] — Optional model override.
 * @returns {Promise<Object>} Response body (may contain { error }).
 */
export async function launchSession(sessionId, provider, model) {
  const body = { sessionId, provider };
  if (model) body.model = model;
  const res = await fetch("/api/launch", {
    method: "POST",
    headers: JSON_HEADERS,
    body: JSON.stringify(body),
  });
  return res.json();
}

/**
 * DELETE /api/sessions — Delete a session.
 * @param {string} sessionId
 * @param {string} provider
 * @returns {Promise<Object>} Response body (may contain { error }).
 */
export async function deleteSession(sessionId, provider) {
  const res = await fetch("/api/sessions", {
    method: "DELETE",
    headers: JSON_HEADERS,
    body: JSON.stringify({ sessionId, provider }),
  });
  return res.json();
}

/**
 * POST /api/new — Create a new project and launch Claude Code.
 * @param {string} dir — Absolute path for the new project directory.
 * @returns {Promise<Object>} Response body (may contain { error }).
 */
export async function createProject(dir, tool, model) {
  const body = { dir };
  if (tool) body.tool = tool;
  if (model) body.model = model;
  const res = await fetch("/api/new", {
    method: "POST",
    headers: JSON_HEADERS,
    body: JSON.stringify(body),
  });
  return res.json();
}

export async function testSSH(host, user, port) {
  const res = await fetch("/api/test-ssh", {
    method: "POST",
    headers: JSON_HEADERS,
    body: JSON.stringify({ host, user, port: port || 22 }),
  });
  return res.json();
}

export async function startSecretScan() {
  const res = await fetch("/api/scan-secrets", { method: "POST" });
  return res.json();
}

export async function getScanStatus() {
  const res = await fetch("/api/scan-secrets");
  return res.json();
}

export async function fetchInventory(forceRefresh = false) {
  const url = forceRefresh ? "/api/inventory?refresh=true" : "/api/inventory";
  const res = await fetch(url);
  if (!res.ok) throw new Error(`Failed to fetch inventory: ${res.statusText}`);
  return res.json();
}

export async function fetchStats() {
  const res = await fetch("/api/stats");
  if (!res.ok) throw new Error(`Failed to fetch stats: ${res.statusText}`);
  return res.json();
}

export async function fetchInventoryFile(path) {
  const res = await fetch(`/api/inventory/file?path=${encodeURIComponent(path)}`);
  if (!res.ok) throw new Error(`Failed to read file: ${res.statusText}`);
  return res.json();
}
