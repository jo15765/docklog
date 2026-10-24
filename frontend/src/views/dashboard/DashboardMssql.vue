<template>
  <div class="mssql-dashboard">
    <section class="page-hero compact-hero">
      <div class="page-hero-body">
        <div class="page-hero-copy">
          <span class="page-hero-eyebrow">Log stream</span>
          <h1>SQL Server logs</h1>
          <p class="page-hero-sub">
            Reading from <code>container_logs.dbo.container_logs</code>. Docker container APIs are not used in this
            runtime mode.
          </p>
        </div>
        <div class="hero-actions">
          <router-link to="/mssql-logs" class="page-btn primary">Open SQL Logs</router-link>
          <router-link v-if="isAdmin" to="/mssql-settings" class="page-btn secondary">MS SQL Settings</router-link>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup>
import { computed } from "vue";
import { sharedState } from "../../utils/sharedState";

const isAdmin = computed(() => sharedState.currentUser?.is_admin === true);
</script>

<style scoped>
.mssql-dashboard {
  padding: 0 0 2rem;
}
.hero-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
  align-items: center;
}
</style>
