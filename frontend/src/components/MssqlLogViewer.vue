<template>
  <div :class="['mssql-log-viewer', 'log-viewer', 'glass', { fullscreen: isFullScreen }]">
    <div class="viewer-header">
      <div class="header-left">
        <div class="status-pulse status-running"></div>
        <div class="name-group">
          <span class="c-name">{{ props.containerName || "All containers" }}</span>
          <span class="c-id">{{ statusLine }}</span>
        </div>
      </div>
      <div class="header-right">
        <div class="tz-control glass">
          <label class="tz-label" for="mssql-tz-select">
            <span class="tz-icon" aria-hidden="true"></span>
            Time zone
          </label>
          <select id="mssql-tz-select" v-model="timezonePreset" class="tz-select" @change="onPresetChange">
            <option v-for="opt in TIMEZONE_PRESETS" :key="opt.value" :value="opt.value">
              {{ opt.label }}
            </option>
            <option :value="CUSTOM_TZ_PRESET">Custom IANA…</option>
          </select>
          <input
            v-if="timezonePreset === CUSTOM_TZ_PRESET"
            v-model="customTimezone"
            type="text"
            class="tz-custom"
            placeholder="e.g. Europe/Berlin"
            spellcheck="false"
            @change="persistTimezone"
            @keydown.enter="persistTimezone"
          />
          <span v-if="timezonePreset === CUSTOM_TZ_PRESET && customTzError" class="tz-error">{{ customTzError }}</span>
        </div>
        <input v-model="logSearchQuery" type="text" placeholder="Search loaded logs…" class="search-input glass" />
        <button type="button" class="btn-ghost" @click="isFullScreen = !isFullScreen">Fullscreen</button>
        <button type="button" class="btn-ghost" :disabled="loadingInitial || loadingBatch" @click="reloadFromStart">
          Reload
        </button>
        <button type="button" class="btn-ghost" @click="downloadLogs">Download</button>
      </div>
    </div>

    <div v-if="totalUIPages > 1" class="pagination-bar">
      <button type="button" class="page-nav" :disabled="navBusy || currentUIPage <= 1" @click="goToUIPage(currentUIPage - 1)">
        ← Newer
      </button>
      <span class="page-info">
        Page <strong>{{ currentUIPage }}</strong> of <strong>{{ totalUIPages }}</strong>
        <span class="page-meta">{{ UI_PAGE_SIZE }} lines · batches of {{ API_BATCH_SIZE }} from SQL</span>
      </span>
      <button
        type="button"
        class="page-nav"
        :disabled="navBusy || currentUIPage >= totalUIPages"
        @click="goToUIPage(currentUIPage + 1)"
      >
        Older →
      </button>
    </div>

    <div ref="logContainer" class="log-container">
      <p v-if="loadingInitial" class="history-loader">{{ loadMessage }}</p>
      <p v-else-if="loadingBatch" class="history-loader">Fetching next {{ API_BATCH_SIZE.toLocaleString() }} rows from SQL…</p>
      <p v-else-if="loadError" class="history-loader error-text">{{ loadError }}</p>
      <div
        v-for="(line, idx) in displayLines"
        :key="`${currentUIPage}-${idx}-${line.time}-${(line.body || '').slice(0, 48)}`"
        :class="['log-line', { 'log-line-alt': idx % 2 === 1 }]"
      >
        <span v-if="line.time" class="line-time">{{ line.time }}</span>
        <span class="line-body">{{ line.body }}</span>
      </div>
      <p v-if="!loadingInitial && !loadError && displayLines.length === 0" class="history-loader">No log rows for this filter.</p>
    </div>

    <div v-if="totalUIPages > 1" class="pagination-bar pagination-bar-bottom">
      <button type="button" class="page-nav" :disabled="navBusy || currentUIPage <= 1" @click="goToUIPage(currentUIPage - 1)">
        ← Newer
      </button>
      <span class="page-info">Page {{ currentUIPage }} / {{ totalUIPages }}</span>
      <button
        type="button"
        class="page-nav"
        :disabled="navBusy || currentUIPage >= totalUIPages"
        @click="goToUIPage(currentUIPage + 1)"
      >
        Older →
      </button>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { createAuthenticatedWebSocket } from "../utils/wsAuth";
import { apiFetch } from "../utils/apiFetch";
import { secureStorage } from "../utils/storage";
import {
  CUSTOM_TZ_PRESET,
  MSSQL_TZ_PRESET_KEY,
  MSSQL_TZ_STORAGE_KEY,
  TIMEZONE_PRESETS,
  formatTimestampInZone,
  isValidTimezone,
  normalizeLogEntry,
  timezoneShortLabel,
} from "../utils/mssqlLogFormat";

const API_BATCH_SIZE = 10000;
const UI_PAGE_SIZE = 100;
const DEFAULT_TZ = "America/New_York";

const props = defineProps({
  containerName: { type: String, default: "" },
});

const logEntries = ref([]);
const logSearchQuery = ref("");
const isFullScreen = ref(false);
const loadingInitial = ref(false);
const loadingBatch = ref(false);
const loadMessage = ref("Loading logs…");
const loadError = ref("");
const totalInDB = ref(0);
const currentUIPage = ref(1);
const minLoadedId = ref(null);
const maxRowId = ref(null);
const hasMoreOlderInDB = ref(false);

const timezonePreset = ref(DEFAULT_TZ);
const customTimezone = ref("");
const customTzError = ref("");
const activeTimezone = ref(DEFAULT_TZ);

let socket = null;
let reconnectTimer = null;

const navBusy = computed(() => loadingInitial.value || loadingBatch.value);

const totalUIPages = computed(() => Math.max(1, Math.ceil(totalInDB.value / UI_PAGE_SIZE)));

const tzDisplayLabel = computed(() => {
  const preset = TIMEZONE_PRESETS.find((p) => p.value === activeTimezone.value);
  return preset ? preset.label : activeTimezone.value;
});

const formatLogBody = (entry) => {
  let body = entry.message ?? "";
  if (entry.container) body = `${entry.container} | ${body}`;
  if (entry.stream) body = `[${entry.stream}] ${body}`;
  return body;
};

const formatLineParts = (entry) => {
  const body = formatLogBody(entry);
  if (!entry.ts) {
    return { time: "", body };
  }
  const stamp = formatTimestampInZone(entry.ts, activeTimezone.value);
  const label = timezoneShortLabel(activeTimezone.value);
  return { time: `${stamp} ${label}`, body };
};

const formattedRows = computed(() => logEntries.value.map((e) => ({ entry: e, parts: formatLineParts(e) })));

const displayLines = computed(() => {
  const q = logSearchQuery.value.trim().toLowerCase();
  let rows = formattedRows.value;
  if (q) {
    rows = rows.filter(({ parts, entry }) => {
      const hay = `${parts.time} ${parts.body} ${entry.message}`.toLowerCase();
      return hay.includes(q);
    });
  }
  const start = (currentUIPage.value - 1) * UI_PAGE_SIZE;
  return rows.slice(start, start + UI_PAGE_SIZE).map((r) => r.parts);
});

const statusLine = computed(() => {
  if (loadingInitial.value) return loadMessage.value;
  const scope = props.containerName?.trim()
    ? `container “${props.containerName.trim()}”`
    : "all containers";
  const loaded = logEntries.value.length;
  const total = totalInDB.value;
  let batchNote = "";
  if (loaded < total) {
    batchNote = ` · ${loaded.toLocaleString()} loaded (${API_BATCH_SIZE.toLocaleString()}/batch)`;
  }
  return `${total.toLocaleString()} in SQL · page ${currentUIPage.value}/${totalUIPages.value}${batchNote} · ${scope} · ${tzDisplayLabel.value}`;
});

const authHeaders = () => ({
  Authorization: `Bearer ${secureStorage.getItem("token")}`,
});

const containerQuery = computed(() => {
  const name = (props.containerName || "").trim();
  return name ? `&container=${encodeURIComponent(name)}` : "";
});

const entriesFromPayload = (payload) => {
  if (payload?.entries?.length) {
    return payload.entries.map(normalizeLogEntry).filter(Boolean);
  }
  return [];
};

const mergeBatchPayload = (payload, replace) => {
  const entries = entriesFromPayload(payload);
  if (!entries.length && !payload?.lines?.length) return;
  if (replace) {
    logEntries.value = entries;
  } else {
    logEntries.value = [...logEntries.value, ...entries];
  }
  if (payload.min_id != null) minLoadedId.value = payload.min_id;
  if (payload.max_id != null && replace) maxRowId.value = payload.max_id;
  if (payload.total != null) totalInDB.value = payload.total;
  hasMoreOlderInDB.value = !!payload.has_more_older;
};

const parseIncomingLine = (trimmed) => {
  if (trimmed.startsWith("{")) {
    try {
      return normalizeLogEntry(JSON.parse(trimmed));
    } catch {
      return null;
    }
  }
  return normalizeLogEntry({ message: trimmed });
};

const loadTimezonePrefs = () => {
  const storedPreset = localStorage.getItem(MSSQL_TZ_PRESET_KEY);
  const storedTz = localStorage.getItem(MSSQL_TZ_STORAGE_KEY);
  if (storedPreset === CUSTOM_TZ_PRESET && storedTz && isValidTimezone(storedTz)) {
    timezonePreset.value = CUSTOM_TZ_PRESET;
    customTimezone.value = storedTz;
    activeTimezone.value = storedTz;
    return;
  }
  if (storedPreset && (storedPreset === CUSTOM_TZ_PRESET || TIMEZONE_PRESETS.some((p) => p.value === storedPreset))) {
    timezonePreset.value = storedPreset;
  } else {
    timezonePreset.value = DEFAULT_TZ;
  }
  if (timezonePreset.value === CUSTOM_TZ_PRESET && storedTz && isValidTimezone(storedTz)) {
    customTimezone.value = storedTz;
    activeTimezone.value = storedTz;
  } else {
    activeTimezone.value = timezonePreset.value;
  }
};

const persistTimezone = () => {
  customTzError.value = "";
  if (timezonePreset.value === CUSTOM_TZ_PRESET) {
    const tz = customTimezone.value.trim();
    if (!isValidTimezone(tz)) {
      customTzError.value = "Invalid IANA zone (e.g. Europe/Berlin)";
      return;
    }
    activeTimezone.value = tz;
    localStorage.setItem(MSSQL_TZ_PRESET_KEY, CUSTOM_TZ_PRESET);
    localStorage.setItem(MSSQL_TZ_STORAGE_KEY, tz);
    return;
  }
  activeTimezone.value = timezonePreset.value;
  localStorage.setItem(MSSQL_TZ_PRESET_KEY, timezonePreset.value);
  localStorage.setItem(MSSQL_TZ_STORAGE_KEY, timezonePreset.value);
};

const onPresetChange = () => {
  customTzError.value = "";
  if (timezonePreset.value !== CUSTOM_TZ_PRESET) {
    persistTimezone();
  }
};

const fetchBatch = async (beforeId = null) => {
  let url = `/api/mssql/logs?tail=${API_BATCH_SIZE}${containerQuery.value}`;
  if (beforeId != null) {
    url += `&before_id=${encodeURIComponent(String(beforeId))}`;
  }
  const res = await apiFetch(url, { headers: authHeaders() });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(body.error || "Failed to load log batch from SQL.");
  }
  return body;
};

const ensureLoadedForUIPage = async (page) => {
  const neededLines = page * UI_PAGE_SIZE;
  while (logEntries.value.length < neededLines && hasMoreOlderInDB.value) {
    if (minLoadedId.value == null) break;
    loadingBatch.value = true;
    try {
      const payload = await fetchBatch(minLoadedId.value);
      const added = payload.entries?.length ?? payload.lines?.length ?? 0;
      if (added === 0) {
        hasMoreOlderInDB.value = false;
        break;
      }
      mergeBatchPayload(payload, false);
      if (!payload.has_more_older || added < API_BATCH_SIZE) {
        hasMoreOlderInDB.value = false;
      }
    } finally {
      loadingBatch.value = false;
    }
  }
};

const goToUIPage = async (page) => {
  const target = Math.min(Math.max(1, page), totalUIPages.value);
  loadError.value = "";
  try {
    await ensureLoadedForUIPage(target);
    currentUIPage.value = target;
    if (target === 1) {
      connectLive();
    } else {
      disconnectLive();
    }
  } catch (e) {
    loadError.value = e.message || String(e);
  }
};

const disconnectLive = () => {
  if (socket) {
    socket.onclose = null;
    socket.close();
    socket = null;
  }
  if (reconnectTimer) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
};

const connectLive = () => {
  disconnectLive();
  const params = new URLSearchParams();
  if (maxRowId.value != null) {
    params.set("after_id", String(maxRowId.value));
  }
  if (props.containerName?.trim()) {
    params.set("container", props.containerName.trim());
  }
  const qs = params.toString() ? `?${params.toString()}` : "";
  socket = createAuthenticatedWebSocket(`/ws/mssql/logs${qs}`);
  socket.onmessage = (e) => {
    if (currentUIPage.value !== 1) return;
    const incoming = [];
    String(e.data || "")
      .split("\n")
      .forEach((line) => {
        const trimmed = line.trimEnd();
        if (!trimmed) return;
        const entry = parseIncomingLine(trimmed);
        if (entry) incoming.push(entry);
      });
    if (incoming.length === 0) return;
    logEntries.value = [...incoming, ...logEntries.value];
    totalInDB.value += incoming.length;
    if (incoming[0]?.id != null) {
      maxRowId.value = Math.max(maxRowId.value ?? 0, ...incoming.map((x) => x.id || 0));
    }
  };
  socket.onclose = () => {
    if (currentUIPage.value === 1) {
      reconnectTimer = setTimeout(connectLive, 2000);
    }
  };
};

const reloadFromStart = async () => {
  loadingInitial.value = true;
  loadError.value = "";
  loadMessage.value = "Loading newest batch from SQL…";
  disconnectLive();
  currentUIPage.value = 1;
  minLoadedId.value = null;
  hasMoreOlderInDB.value = false;
  try {
    const payload = await fetchBatch(null);
    mergeBatchPayload(payload, true);
    connectLive();
  } catch (e) {
    loadError.value = e.message || String(e);
    logEntries.value = [];
  } finally {
    loadingInitial.value = false;
  }
};

const downloadLogs = async () => {
  const res = await apiFetch(`/api/mssql/logs/download?x=1${containerQuery.value}`, { headers: authHeaders() });
  if (!res.ok) return;
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = "mssql-logs.log";
  a.click();
  URL.revokeObjectURL(url);
};

watch(
  () => props.containerName,
  () => {
    logSearchQuery.value = "";
    reloadFromStart();
  },
);

onMounted(() => {
  loadTimezonePrefs();
  reloadFromStart();
});

onUnmounted(() => {
  disconnectLive();
});
</script>

<style scoped>
.mssql-log-viewer {
  display: flex;
  flex-direction: column;
  min-height: 60vh;
  border-radius: 16px;
  overflow: hidden;
}
.viewer-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 1rem;
  padding: 0.75rem 1rem;
  border-bottom: 1px solid var(--border-subtle);
}
.header-left {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  min-width: 0;
}
.name-group {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.c-name {
  font-weight: 700;
}
.c-id {
  font-size: 0.75rem;
  color: var(--text-muted);
  max-width: 36rem;
  line-height: 1.35;
}
.header-right {
  display: flex;
  align-items: flex-end;
  gap: 0.5rem;
  flex-wrap: wrap;
  justify-content: flex-end;
}
.tz-control {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  padding: 0.45rem 0.65rem;
  border-radius: 10px;
  border: 1px solid var(--border-subtle);
  background: rgba(8, 145, 178, 0.06);
  min-width: 11rem;
}
.tz-label {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.65rem;
  font-weight: 700;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--text-muted);
}
.tz-icon {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: rgba(8, 145, 178, 0.85);
  box-shadow: 0 0 6px rgba(8, 145, 178, 0.45);
}
.tz-select {
  appearance: none;
  padding: 0.4rem 1.75rem 0.4rem 0.55rem;
  border-radius: 8px;
  border: 1px solid var(--border-subtle);
  background: var(--bg-input, rgba(0, 0, 0, 0.2))
    url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='%2394a3b8' stroke-width='2'%3E%3Cpath d='M6 9l6 6 6-6'/%3E%3C/svg%3E")
    no-repeat right 0.5rem center;
  color: var(--text-main, inherit);
  font-size: 0.8rem;
  cursor: pointer;
}
.tz-custom {
  padding: 0.35rem 0.5rem;
  border-radius: 8px;
  border: 1px solid var(--border-subtle);
  background: var(--bg-input, transparent);
  font-size: 0.78rem;
  font-family: ui-monospace, monospace;
}
.tz-error {
  font-size: 0.68rem;
  color: var(--danger, #f87171);
}
.search-input {
  padding: 0.4rem 0.65rem;
  border-radius: 8px;
  border: 1px solid var(--border-subtle);
  min-width: 160px;
}
.btn-ghost {
  padding: 0.35rem 0.65rem;
  border-radius: 8px;
  border: 1px solid var(--border-subtle);
  background: transparent;
  cursor: pointer;
}
.btn-ghost:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.pagination-bar {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 1rem;
  flex-wrap: wrap;
  padding: 0.5rem 1rem;
  border-bottom: 1px solid var(--border-subtle);
  background: rgba(148, 163, 184, 0.06);
}
.pagination-bar-bottom {
  border-bottom: none;
  border-top: 1px solid var(--border-subtle);
}
.page-nav {
  padding: 0.35rem 0.75rem;
  border-radius: 8px;
  border: 1px solid var(--border-subtle);
  background: var(--bg-card, transparent);
  cursor: pointer;
  font-size: 0.85rem;
}
.page-nav:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
.page-info {
  font-size: 0.85rem;
  color: var(--text-muted);
  text-align: center;
}
.page-meta {
  display: block;
  font-size: 0.72rem;
  margin-top: 0.15rem;
}
.log-container {
  flex: 1;
  overflow: auto;
  padding: 0.75rem 1rem;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  line-height: 1.45;
}
.log-line {
  display: flex;
  gap: 0.75rem;
  align-items: baseline;
  word-break: break-word;
  padding: 3px 8px;
  margin: 0 -8px;
  border-radius: 4px;
}
.line-time {
  flex-shrink: 0;
  color: rgba(8, 145, 178, 0.95);
  font-size: 0.72rem;
  white-space: nowrap;
  user-select: none;
}
.line-body {
  flex: 1;
  min-width: 0;
  white-space: pre-wrap;
}
.log-line-alt {
  background: rgba(148, 163, 184, 0.12);
}
:root[data-theme="light"] .mssql-log-viewer .log-line-alt {
  background: rgba(15, 23, 42, 0.05);
}
.history-loader {
  color: var(--text-muted);
  font-size: 0.8rem;
  margin-bottom: 0.5rem;
}
.error-text {
  color: var(--danger, #f87171);
}
.status-pulse {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--success);
}
.fullscreen {
  position: fixed;
  inset: 0;
  z-index: 1000;
  min-height: 100vh;
  border-radius: 0;
}
</style>
