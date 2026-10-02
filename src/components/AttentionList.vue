<script setup lang="ts">
import { faCircleCheck } from '@fortawesome/free-solid-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import { computed, ref } from 'vue'
import FilterBar from '@/components/FilterBar.vue'
import RepoGroup from '@/components/RepoGroup.vue'
import { useColumns } from '@/composables/useColumns'
import { balanceColumns } from '@/lib/layout'
import type { Facets, Filters } from '@/lib/filters'
import type { Block } from '@/lib/types'

const props = defineProps<{
  blocks: Block[]
  nowMs: number
  critSeconds: number
  facets: Facets
  owners: string[]
  active: number
  manualRefresh?: boolean
  refreshing?: string[]
  scanning?: boolean
}>()
const filters = defineModel<Filters>('filters', { required: true })
defineEmits<{ clear: []; refresh: [fullName: string] }>()

const columnCount = useColumns()
// A group's height is its header plus its rows, so rows+2 approximates it.
const columns = computed(() =>
  balanceColumns(props.blocks, columnCount.value, (b) => b.rows.length + 2),
)

const bar = ref<InstanceType<typeof FilterBar> | null>(null)
defineExpose({ focusSearch: () => bar.value?.focus() })
</script>

<template>
  <section aria-labelledby="attention-title">
    <h2 id="attention-title" class="mb-4 text-lg font-semibold tracking-tight">Needs attention</h2>
    <FilterBar
      ref="bar"
      v-model:filters="filters"
      :facets="facets"
      :owners="owners"
      :active="active"
      class="mb-5"
      @clear="$emit('clear')"
    />

    <div
      v-if="blocks.length === 0"
      class="border-border text-muted-foreground flex items-center gap-3 rounded-lg border border-dashed p-6"
      data-test="empty"
    >
      <FontAwesomeIcon :icon="faCircleCheck" class="text-ok text-xl" aria-hidden="true" />
      Nothing matches the current filters.
    </div>
    <div v-else class="flex items-start gap-4" data-test="columns">
      <div v-for="(column, i) in columns" :key="i" class="flex min-w-0 flex-1 flex-col gap-4">
        <RepoGroup
          v-for="block in column"
          :key="block.repo.fullName"
          :block="block"
          :now-ms="nowMs"
          :crit-seconds="critSeconds"
          :manual-refresh="manualRefresh"
          :refreshing="refreshing"
          :scanning="scanning"
          @refresh="(name) => $emit('refresh', name)"
        />
      </div>
    </div>
  </section>
</template>
