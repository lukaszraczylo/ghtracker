<script setup lang="ts">
import { faCircleCheck } from '@fortawesome/free-solid-svg-icons'
import { EmptyState, Heading } from '@perfect-design/ui'
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
    <Heading id="attention-title" :level="2" size="lg" class="mb-4">Needs attention</Heading>
    <FilterBar
      ref="bar"
      v-model:filters="filters"
      :facets="facets"
      :owners="owners"
      :active="active"
      class="mb-5"
      @clear="$emit('clear')"
    />

    <EmptyState
      v-if="blocks.length === 0"
      :icon="faCircleCheck"
      tone="success"
      variant="bordered-dashed"
      layout="start"
      size="sm"
      title="Nothing matches the current filters."
      title-as="p"
      data-test="empty"
    />
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
