<template>
  <div class="mssql-logs-layout" :class="{ 'sidebar-collapsed': sidebarHidden }">
    <aside v-if="configured" class="containers-sidebar glass">
      <div class="sidebar-head">
        <span class="label-caps">Containers</span>
        <button type="button" class="mini-btn" @click="sidebarHidden = true">Hide</button>
      </div>
      <button
        type="button"
        class="container-item"
        :class="{ active: !selectedContainer }"
        @click="selectedContainer = ''"
      >
        All containers
      </button>
      <button
        v-for="name in containers"
        :key="name"
        type="button"
        class="container-item"
        :class="{ active: selectedContainer === name }"
        @click="selectedContainer = name"
      >
        {{ name }}
      </button>
    </aside>

    <div class="logs-main">
      <section class="page-hero compact-hero">
        <div class="page-hero-body">
          <div class="page-hero-copy">
            <span class="page-hero-eyebrow">Log stream</span>
            <h1>{{ selectedContainer || "All containers" }}</h1>
            <p class="page-hero-sub" v-if="configured">From <code>container_logs.dbo.container_logs</code></p>
            <p class="page-hero-sub warn" v-else>
              MS SQL log source is not configured.
              <router-link v-if="isAdmin" to="/mssql-settings">Open settings</router-link>
            </p>
          </div>
          <button
            v-if="configured && sidebarHidden"
            type="button"
            class="page-btn secondary"
            @click="sidebarHidden = false"
          >
            Show containers
          </button>
        </div>
      </section>

      <MssqlLogViewer v-if="configured" :key="selectedContainer || '__all__'" :container-name="selectedContainer" />
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from "vue";
import MssqlLogViewer from "../components/MssqlLogViewer.vue";
import { apiFetch } from "../utils/apiFetch";
import { sharedState } from "../utils/sharedState";
import { secureStorage } from "../utils/storage";

const configured = ref(false);
const containers = ref([]);
const selectedContainer = ref("");
const sidebarHidden = ref(false);

const isAdmin = computed(() => sharedState.currentUser?.is_admin === true);

const authHeaders = () => ({
  Authorization: `Bearer ${secureStorage.getItem("token")}`,
});

onMounted(async () => {
  try {
    const res = await apiFetch("/api/mssql/status", { headers: authHeaders() });
    if (res.ok) {
      const data = await res.json();
      configured.value = !!data.configured;
    }
    if (configured.value) {
      const listRes = await apiFetch("/api/mssql/containers", { headers: authHeaders() });
      if (listRes.ok) {
        containers.value = await listRes.json();
      }
    }
  } catch {
    configured.value = false;
  }
});
</script>

<style scoped>
.mssql-logs-layout {
  display: grid;
  grid-template-columns: 260px 1fr;
  gap: 1rem;
  min-height: calc(100vh - 120px);
}
.mssql-logs-layout.sidebar-collapsed {
  grid-template-columns: 1fr;
}
.containers-sidebar {
  border-radius: 16px;
  padding: 0.75rem;
  max-height: calc(100vh - 120px);
  overflow: auto;
}
.sidebar-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.75rem;
}
.container-item {
  display: block;
  width: 100%;
  text-align: left;
  padding: 0.5rem 0.65rem;
  margin-bottom: 0.25rem;
  border: none;
  border-radius: 8px;
  background: transparent;
  cursor: pointer;
  font-size: 0.875rem;
}
.container-item.active,
.container-item:hover {
  background: var(--surface-2);
}
.compact-hero {
  margin-bottom: 1rem;
}
.warn a {
  color: var(--primary);
  margin-left: 0.35rem;
}
.mini-btn {
  font-size: 0.75rem;
  border: none;
  background: transparent;
  cursor: pointer;
  color: var(--text-muted);
}
@media (max-width: 900px) {
  .mssql-logs-layout {
    grid-template-columns: 1fr;
  }
  .containers-sidebar {
    max-height: 200px;
  }
}
</style>
