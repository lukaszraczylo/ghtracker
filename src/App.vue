<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import AppHeader from '@/components/AppHeader.vue'
import AttentionList from '@/components/AttentionList.vue'
import FleetOverview from '@/components/FleetOverview.vue'
import ProjectList from '@/components/ProjectList.vue'
import { filtersFromSearch, filtersToSearch } from '@/lib/filters'
import { useTrackerStore } from '@/stores/tracker'

const POLL_MS = 60_000
const CLOCK_MS = 30_000

const store = useTrackerStore()
const {
  state,
  error,
  notice,
  scanning,
  blocks,
  visibleBlocks,
  nowMs,
  filters,
  facets,
  owners,
  visibleRepos,
  activeFilters,
  repoCounts,
  facts,
} = storeToRefs(store)
const list = ref<InstanceType<typeof AttentionList> | null>(null)
const matching = computed(() => new Set(visibleRepos.value.map((r) => r.fullName)))
let timers: number[] = []

/** Pressing "/" outside a text field jumps to the search box. */
function onKey(e: KeyboardEvent): void {
  const el = e.target as HTMLElement | null
  if (e.key !== '/' || e.metaKey || e.ctrlKey || el?.closest('input, textarea, [contenteditable]'))
    return
  e.preventDefault()
  list.value?.focusSearch()
}

// Filters live in the URL so a filtered view can be shared and survives a reload.
watch(
  filters,
  (f) => window.history.replaceState(null, '', window.location.pathname + filtersToSearch(f)),
  { deep: true },
)

onMounted(() => {
  filters.value = filtersFromSearch(window.location.search)
  window.addEventListener('keydown', onKey)
  void store.load()
  timers = [
    window.setInterval(() => void store.load(), POLL_MS),
    window.setInterval(store.advanceClock, CLOCK_MS),
  ]
})
onBeforeUnmount(() => {
  timers.forEach((t) => window.clearInterval(t))
  window.removeEventListener('keydown', onKey)
})
</script>

<template>
  <main class="w-full px-4 py-4 sm:px-6 sm:py-6 lg:px-10">
    <AppHeader
      :last-refresh="state?.lastRefresh ?? ''"
      :now-ms="nowMs"
      :interval-seconds="state?.intervalSeconds ?? 0"
      :loaded="state?.loaded ?? false"
      :scanning="scanning"
      :manual-refresh="state?.manualRefresh ?? false"
      @rescan="store.rescan()"
    />

    <p v-if="error" role="alert" class="text-crit mt-4 text-sm" data-test="error">
      Could not reach the server: {{ error }}
    </p>

    <p v-if="notice" role="status" class="text-warn mt-4 text-sm" data-test="notice">
      {{ notice }}
    </p>

    <p v-if="!state" class="text-muted-foreground mt-16 text-3xl font-semibold">Loading</p>
    <p
      v-else-if="!state.loaded"
      class="text-muted-foreground mt-16 text-3xl font-semibold"
      data-test="waiting"
    >
      Fetching data from GitHub
    </p>
    <template v-else>
      <div class="mt-8 sm:mt-12">
        <FleetOverview
          :crit="state.counts.crit"
          :warn="state.counts.warn"
          :block-count="blocks.length"
          :repos="state.repos"
          :counts="repoCounts"
          :facts="facts!"
          :focused="filters.repo"
          :matching="matching"
          @select="store.toggleRepo"
        />
      </div>
      <div
        class="mt-10 grid grid-cols-[minmax(0,1fr)] gap-x-10 gap-y-10 sm:mt-14 lg:grid-cols-[minmax(0,1fr)_24rem] 2xl:grid-cols-[minmax(0,1fr)_28rem]"
      >
        <AttentionList
          ref="list"
          v-model:filters="filters"
          :blocks="visibleBlocks"
          :now-ms="nowMs"
          :crit-seconds="state.thresholds.prCritSeconds"
          :facets="facets"
          :owners="owners"
          :active="activeFilters"
          @clear="store.clearFilters()"
        />
        <aside class="lg:sticky lg:top-6 lg:self-start">
          <ProjectList
            :repos="visibleRepos"
            :total="state.repos.length"
            :focused="filters.repo"
            :now-ms="nowMs"
            @select="store.toggleRepo"
          />
        </aside>
      </div>
    </template>
  </main>
</template>
