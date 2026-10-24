<template>
  <div class="page-view mssql-settings-view animate-fade-in">
    <section class="page-hero">
      <div class="page-hero-body">
        <div class="page-hero-copy">
          <span class="page-hero-eyebrow">Administration</span>
          <div class="page-hero-title-row">
            <span class="page-hero-mark sql-mark" aria-hidden="true">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <ellipse cx="12" cy="5" rx="9" ry="3"></ellipse>
                <path d="M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5"></path>
                <path d="M3 12c0 1.66 4 3 9 3s9-1.34 9-3"></path>
              </svg>
            </span>
            <h1>SQL Server connection</h1>
          </div>
          <p class="page-hero-sub">
            Connect DockLog to your SQL Server where container logs are stored. You only enter server login details here —
            the app already knows which database and table to read.
          </p>
        </div>
        <div class="page-hero-stats">
          <div class="page-hero-stat" :class="form.enabled ? 'success' : 'muted'">
            <span class="page-hero-stat-val">{{ form.enabled ? "On" : "Off" }}</span>
            <span class="page-hero-stat-lbl">Log streaming</span>
          </div>
          <div class="page-hero-stat" :class="connectionStatClass">
            <span class="page-hero-stat-val">{{ connectionStatLabel }}</span>
            <span class="page-hero-stat-lbl">Connection</span>
          </div>
          <div class="page-hero-stat" :class="isReady ? 'success' : dirty ? 'warning' : 'muted'">
            <span class="page-hero-stat-val">{{ isReady ? "Go" : dirty ? "Save" : "Setup" }}</span>
            <span class="page-hero-stat-lbl">View logs</span>
          </div>
        </div>
      </div>
      <div class="page-hero-mesh" aria-hidden="true"></div>
    </section>

    <div v-if="loading" class="loading-state">Loading SQL Server settings…</div>

    <form v-else class="mssql-form" @submit.prevent="save">
      <div class="mssql-layout">
        <aside class="mssql-sidebar page-panel flush">
          <div class="sidebar-block">
            <h3>Step 1 — Turn it on</h3>
            <label class="master-toggle">
              <input v-model="form.enabled" type="checkbox" class="master-toggle-input" @change="markDirty" />
              <span class="master-toggle-copy">
                <span class="master-toggle-title">Stream logs from SQL Server</span>
                <span class="master-toggle-hint">
                  When this is on and saved, the SQL Logs page shows live data from your table.
                </span>
              </span>
            </label>
            <p v-if="!form.enabled" class="callout warn">
              Log streaming is off. Turn on the switch above, then save your connection details.
            </p>
          </div>

          <div class="sidebar-block">
            <h3>Setup checklist</h3>
            <ul class="checklist">
              <li :class="{ ok: form.enabled }">Log streaming turned on</li>
              <li :class="{ ok: hasHost }">Server host entered</li>
              <li :class="{ ok: hasUsername }">SQL username entered</li>
              <li :class="{ ok: hasPassword }">SQL password saved</li>
              <li :class="{ ok: lastTestOk }">Connection test passed (recommended)</li>
              <li :class="{ ok: isReady && !dirty }">{{ dirty ? "Save your changes" : "Settings saved" }}</li>
            </ul>
          </div>

          <div class="sidebar-block">
            <h3>Where logs come from</h3>
            <p class="sidebar-note">
              DockLog reads your existing Docker log table automatically. You do not pick a database or table name in this
              screen.
            </p>
            <div class="schema-pills">
              <span class="schema-pill"><strong>Database</strong> container_logs</span>
              <span class="schema-pill"><strong>Table</strong> dbo.container_logs</span>
            </div>
          </div>

          <router-link
            v-if="isReady"
            to="/mssql-logs"
            class="page-btn primary full-width"
          >
            Open SQL Logs
          </router-link>
          <button
            v-else
            type="button"
            class="page-btn secondary full-width"
            disabled
            title="Complete and save setup first"
          >
            Open SQL Logs
          </button>
        </aside>

        <div class="mssql-main page-panel">
          <div class="editor-header">
            <div class="editor-icon">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                <rect x="2" y="3" width="20" height="14" rx="2"></rect>
                <path d="M8 21h8"></path>
                <path d="M12 17v4"></path>
              </svg>
            </div>
            <div>
              <h2>Server login</h2>
              <p>These are your Microsoft SQL Server credentials — not your DockLog admin password.</p>
            </div>
          </div>

          <div class="steps-banner">
            <div class="step-chip"><span>2</span> Fill in host &amp; login</div>
            <div class="step-chip"><span>3</span> Test connection</div>
            <div class="step-chip"><span>4</span> Save settings</div>
          </div>

          <div class="form-section">
            <h3 class="section-label">Server</h3>
            <div class="form-grid">
              <div class="input-group">
                <label for="mssql-host">Host / address</label>
                <input
                  id="mssql-host"
                  v-model="form.host"
                  class="premium-input"
                  placeholder="e.g. localhost, 192.168.1.10, sql.mycompany.com"
                  required
                  @input="markDirty"
                />
                <span class="field-hint">Hostname or IP of the machine running SQL Server.</span>
              </div>
              <div class="input-group">
                <label for="mssql-port">Port</label>
                <input
                  id="mssql-port"
                  v-model.number="form.port"
                  type="number"
                  class="premium-input"
                  placeholder="1433"
                  required
                  @input="markDirty"
                />
                <span class="field-hint">Default is 1433 unless your DBA changed it.</span>
              </div>
            </div>
          </div>

          <div class="form-section">
            <h3 class="section-label">Authentication</h3>
            <div class="form-grid">
              <div class="input-group">
                <label for="mssql-user">Username</label>
                <input
                  id="mssql-user"
                  v-model="form.username"
                  class="premium-input"
                  placeholder="SQL login"
                  autocomplete="off"
                  required
                  @input="markDirty"
                />
              </div>
              <div class="input-group">
                <label for="mssql-pass">Password</label>
                <input
                  id="mssql-pass"
                  v-model="form.password"
                  type="password"
                  class="premium-input"
                  :placeholder="form.password_set ? 'Saved — leave blank to keep' : 'Enter SQL password'"
                  autocomplete="new-password"
                  @input="markDirty"
                />
                <span v-if="form.password_set && !form.password" class="field-hint ok-hint">
                  A password is already stored securely. Type here only if you want to change it.
                </span>
              </div>
            </div>
          </div>

          <div class="form-section security-options">
            <h3 class="section-label">Encryption (TLS)</h3>
            <p class="section-intro">
              Most SQL Server installs use encrypted connections. If the test fails with a TLS or handshake error, adjust
              these options.
            </p>
            <label class="option-toggle recommended">
              <input v-model="form.trust_server_certificate" type="checkbox" @change="markDirty" />
              <span>
                <strong>Trust server certificate (Yes)</strong>
                <small>
                  Leave this on for Docker, local SQL, and self-signed certs (fixes TLS / x509 “Server” errors). Turn off
                  only if you use a proper CA-signed cert and need strict verification.
                </small>
              </span>
            </label>
            <label class="option-toggle">
              <input v-model="form.encrypt_disable" type="checkbox" @change="markDirty" />
              <span>
                <strong>Unencrypted connection</strong>
                <small>Only if your server does not use TLS (legacy/internal).</small>
              </span>
            </label>
          </div>

          <div v-if="saveError" class="callout error" role="alert">{{ saveError }}</div>

          <div class="action-bar">
            <p v-if="dirty" class="dirty-note">You have unsaved changes.</p>
            <p v-else-if="isReady" class="dirty-note success-note">Saved and ready — open SQL Logs to view streams.</p>
            <div class="action-buttons">
              <button type="button" class="page-btn secondary" :disabled="saving || testing" @click="testConnection">
                {{ testing ? "Testing…" : "Test connection" }}
              </button>
              <button type="submit" class="page-btn primary" :disabled="saving || testing">
                {{ saving ? "Saving…" : "Save settings" }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </form>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from "vue";
import { apiFetch } from "../utils/apiFetch";
import { showToast } from "../utils/sharedState";
import { secureStorage } from "../utils/storage";

const form = reactive({
  host: "",
  port: 1433,
  username: "",
  password: "",
  enabled: true,
  trust_server_certificate: true,
  encrypt_disable: false,
  password_set: false,
});

const status = reactive({ configured: false, enabled: false });
const loading = ref(true);
const saving = ref(false);
const testing = ref(false);
const dirty = ref(false);
const lastTestOk = ref(false);
const saveError = ref("");

const hasHost = computed(() => !!form.host?.trim());
const hasUsername = computed(() => !!form.username?.trim());
const hasPassword = computed(() => form.password_set || !!form.password?.trim());

const isReady = computed(
  () => form.enabled && status.configured && hasHost.value && hasUsername.value && hasPassword.value,
);

const connectionStatLabel = computed(() => {
  if (!form.enabled) return "Off";
  if (isReady.value) return "Ready";
  if (hasHost.value && hasUsername.value) return "Almost";
  return "Needed";
});

const connectionStatClass = computed(() => {
  if (!form.enabled) return "muted";
  if (isReady.value) return "success";
  if (hasHost.value) return "warning";
  return "muted";
});

const authHeaders = () => ({
  Authorization: `Bearer ${secureStorage.getItem("token")}`,
  "Content-Type": "application/json",
});

const markDirty = () => {
  dirty.value = true;
  saveError.value = "";
};

const refreshStatus = async () => {
  const statusRes = await apiFetch("/api/mssql/status", { headers: authHeaders() });
  if (statusRes.ok) {
    const s = await statusRes.json();
    status.configured = !!s.configured;
    status.enabled = !!s.enabled;
  }
};

const loadSettings = async () => {
  loading.value = true;
  try {
    const res = await apiFetch("/api/admin/mssql-settings", { headers: authHeaders() });
    if (!res.ok) {
      showToast("Could not load settings", "Check that you are signed in as an administrator.", "error");
      return;
    }
    const data = await res.json();
    const firstTime = !data.host && !data.password_set;
    Object.assign(form, {
      host: data.host || "",
      port: data.port || 1433,
      username: data.username || "",
      password: "",
      enabled: firstTime ? true : !!data.enabled,
      trust_server_certificate: data.trust_server_certificate !== false,
      encrypt_disable: !!data.encrypt_disable,
      password_set: !!data.password_set,
    });
    await refreshStatus();
    dirty.value = false;
  } finally {
    loading.value = false;
  }
};

const payloadFromForm = () => ({
  host: form.host.trim(),
  port: form.port,
  username: form.username.trim(),
  password: form.password,
  enabled: form.enabled,
  trust_server_certificate: form.trust_server_certificate,
  encrypt_disable: form.encrypt_disable,
});

const testConnection = async () => {
  if (!hasHost.value || !hasUsername.value) {
    showToast("Missing details", "Enter host and username before testing.", "error");
    return;
  }
  if (!form.password && !form.password_set) {
    showToast("Password required", "Enter the SQL password to test the connection.", "error");
    return;
  }
  testing.value = true;
  saveError.value = "";
  try {
    const res = await apiFetch("/api/admin/mssql-settings/test", {
      method: "POST",
      headers: authHeaders(),
      body: JSON.stringify(payloadFromForm()),
    });
    const body = await res.json().catch(() => ({}));
    if (res.ok) {
      lastTestOk.value = true;
      showToast("Connection successful", "SQL Server accepted these credentials.", "success");
    } else {
      lastTestOk.value = false;
      showToast("Connection failed", body.error || "Could not reach SQL Server.", "error");
    }
  } finally {
    testing.value = false;
  }
};

const save = async () => {
  if (!form.enabled) {
    showToast("Turn streaming on", "Enable “Stream logs from SQL Server” before saving.", "error");
    return;
  }
  if (!form.password && !form.password_set) {
    saveError.value = "Enter the SQL Server password (required the first time you save).";
    return;
  }
  saving.value = true;
  saveError.value = "";
  try {
    const res = await apiFetch("/api/admin/mssql-settings", {
      method: "PUT",
      headers: authHeaders(),
      body: JSON.stringify(payloadFromForm()),
    });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) {
      saveError.value = body.error || "Could not save settings. Try again or check server logs.";
      showToast("Save failed", saveError.value, "error");
      return;
    }
    form.password = "";
    form.password_set = !!body.password_set;
    await refreshStatus();
    status.configured = form.enabled && hasHost.value && hasUsername.value && form.password_set;
    status.enabled = form.enabled;
    dirty.value = false;
    showToast("Settings saved", "You can open SQL Logs from the sidebar.", "success");
  } finally {
    saving.value = false;
  }
};

onMounted(loadSettings);
</script>

<style scoped>
.mssql-form {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.mssql-layout {
  display: grid;
  grid-template-columns: minmax(260px, 320px) minmax(0, 1fr);
  gap: 1.25rem;
  align-items: start;
}

.mssql-sidebar {
  position: sticky;
  top: 1rem;
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}

.sidebar-block {
  display: flex;
  flex-direction: column;
  gap: 0.65rem;
}

.sidebar-block h3 {
  margin: 0;
  font-size: 0.78rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--text-mute);
}

.master-toggle {
  display: flex;
  align-items: flex-start;
  gap: 0.75rem;
  width: 100%;
  margin: 0;
  padding: 1rem;
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  background: linear-gradient(135deg, var(--bg-input) 0%, var(--bg-card) 100%);
  cursor: pointer;
  transition: border-color 0.2s, box-shadow 0.2s;
}

.master-toggle:has(.master-toggle-input:checked) {
  border-color: rgba(var(--accent-rgb), 0.45);
  box-shadow: 0 0 0 1px rgba(var(--accent-rgb), 0.12);
}

.master-toggle-input {
  width: 1.15rem;
  height: 1.15rem;
  margin: 0.15rem 0 0;
  accent-color: var(--accent);
  cursor: pointer;
}

.master-toggle-title {
  font-size: 0.95rem;
  font-weight: 700;
  color: var(--text-main);
  line-height: 1.35;
}

.master-toggle-hint {
  font-size: 0.8rem;
  color: var(--text-mute);
  line-height: 1.45;
}

.master-toggle-copy {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.checklist {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.55rem;
  font-size: 0.84rem;
  color: var(--text-mute);
}

.checklist li {
  position: relative;
  padding-left: 1.35rem;
  line-height: 1.4;
}

.checklist li::before {
  content: "○";
  position: absolute;
  left: 0;
}

.checklist li.ok {
  color: var(--text-main);
}

.checklist li.ok::before {
  content: "✓";
  color: var(--success, #10b981);
  font-weight: 700;
}

.sidebar-note {
  margin: 0;
  font-size: 0.82rem;
  line-height: 1.5;
  color: var(--text-mute);
}

.schema-pills {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.schema-pill {
  display: block;
  padding: 0.65rem 0.75rem;
  border-radius: var(--radius-md);
  background: var(--bg-subtle);
  border: 1px solid var(--border);
  font-size: 0.82rem;
  color: var(--text-main);
}

.schema-pill strong {
  display: block;
  font-size: 0.65rem;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--text-mute);
  margin-bottom: 0.2rem;
}

.callout {
  margin: 0;
  padding: 0.75rem 0.85rem;
  border-radius: var(--radius-md);
  font-size: 0.82rem;
  line-height: 1.45;
}

.callout.warn {
  background: rgba(245, 158, 11, 0.12);
  border: 1px solid rgba(245, 158, 11, 0.35);
  color: var(--text-main);
}

.callout.error {
  background: rgba(239, 68, 68, 0.1);
  border: 1px solid rgba(239, 68, 68, 0.35);
}

.page-hero-title-row {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.page-hero-mark.sql-mark {
  width: 44px;
  height: 44px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--accent-soft);
  color: var(--accent);
  border: 1px solid rgba(var(--accent-rgb), 0.15);
}

.page-hero-mark.sql-mark svg {
  width: 22px;
  height: 22px;
}

.loading-state {
  padding: 2rem;
  text-align: center;
  color: var(--text-mute);
}

.mssql-main {
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}

.editor-header {
  display: flex;
  gap: 1rem;
  align-items: flex-start;
}

.editor-icon {
  width: 48px;
  height: 48px;
  border-radius: 14px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--accent-soft);
  color: var(--accent);
  flex-shrink: 0;
}

.editor-icon svg {
  width: 22px;
  height: 22px;
}

.editor-header h2 {
  margin: 0;
  font-size: 1.25rem;
  font-weight: 800;
}

.editor-header p {
  margin: 0.35rem 0 0;
  font-size: 0.88rem;
  color: var(--text-mute);
  line-height: 1.45;
  max-width: 52ch;
}

.steps-banner {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.step-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.4rem 0.75rem;
  border-radius: 999px;
  font-size: 0.78rem;
  font-weight: 600;
  background: var(--bg-subtle);
  border: 1px solid var(--border);
  color: var(--text-mute);
}

.step-chip span {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1.25rem;
  height: 1.25rem;
  border-radius: 50%;
  background: var(--accent-soft);
  color: var(--accent);
  font-size: 0.7rem;
}

.form-section {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.section-label {
  margin: 0;
  font-size: 0.72rem;
  font-weight: 800;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--text-mute);
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 1rem;
}

.input-group label {
  display: block;
  font-size: 0.8rem;
  font-weight: 700;
  color: var(--text-main);
  margin-bottom: 0.4rem;
}

.field-hint {
  display: block;
  margin-top: 0.35rem;
  font-size: 0.75rem;
  color: var(--text-mute);
  line-height: 1.4;
}

.field-hint.ok-hint {
  color: var(--success, #10b981);
}

.action-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  padding-top: 1rem;
  border-top: 1px solid var(--border);
}

.dirty-note {
  margin: 0;
  font-size: 0.85rem;
  color: var(--text-mute);
}

.dirty-note.success-note {
  color: var(--success, #10b981);
  font-weight: 600;
}

.action-buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 0.65rem;
  margin-left: auto;
}

.security-options {
  padding: 1rem;
  border-radius: var(--radius-lg);
  border: 1px solid var(--border);
  background: var(--bg-subtle);
}

.section-intro {
  margin: 0 0 0.75rem;
  font-size: 0.82rem;
  color: var(--text-mute);
  line-height: 1.45;
}

.option-toggle {
  display: flex;
  align-items: flex-start;
  gap: 0.65rem;
  margin-bottom: 0.75rem;
  cursor: pointer;
}

.option-toggle:last-child {
  margin-bottom: 0;
}

.option-toggle input {
  margin-top: 0.2rem;
  accent-color: var(--accent);
}

.option-toggle strong {
  display: block;
  font-size: 0.88rem;
  color: var(--text-main);
}

.option-toggle small {
  display: block;
  margin-top: 0.15rem;
  font-size: 0.76rem;
  color: var(--text-mute);
  line-height: 1.35;
}

.full-width {
  width: 100%;
  justify-content: center;
  text-align: center;
  text-decoration: none;
}

@media (max-width: 900px) {
  .mssql-layout {
    grid-template-columns: 1fr;
  }

  .mssql-sidebar {
    position: static;
  }

  .form-grid {
    grid-template-columns: 1fr;
  }

  .action-bar {
    flex-direction: column;
    align-items: stretch;
  }

  .action-buttons {
    margin-left: 0;
    flex-direction: column;
  }
}
</style>
