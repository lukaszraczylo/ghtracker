<script setup lang="ts">
import {
  faCircleDot,
  faCodePullRequest,
  faGears,
  faMagnifyingGlass,
  faTriangleExclamation,
  faXmark,
  type IconDefinition,
} from '@fortawesome/free-solid-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import { ref } from 'vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { KIND_GROUPS, type Facets, type Filters, type KindGroup } from '@/lib/filters'
import type { SeverityFilter } from '@/lib/filters'

defineProps<{ facets: Facets; owners: string[]; active: number }>()
const filters = defineModel<Filters>('filters', { required: true })
defineEmits<{ clear: [] }>()

const KIND_META: Record<KindGroup, { label: string; icon: IconDefinition }> = {
  workflow: { label: 'Workflows', icon: faGears },
  pr: { label: 'Pull requests', icon: faCodePullRequest },
  issue: { label: 'Issues', icon: faCircleDot },
  data: { label: 'Data', icon: faTriangleExclamation },
}

const search = ref<InstanceType<typeof Input> | null>(null)
defineExpose({ focus: () => (search.value?.$el as HTMLInputElement | undefined)?.focus() })

function patch(change: Partial<Filters>): void {
  filters.value = { ...filters.value, ...change }
}
function onSeverity(value: unknown): void {
  if (value === 'all' || value === 'crit' || value === 'warn')
    patch({ severity: value as SeverityFilter })
  else patch({ severity: 'all' })
}
function onKinds(value: unknown): void {
  patch({ kinds: (Array.isArray(value) ? value : []) as KindGroup[] })
}
function onOwners(value: unknown): void {
  patch({ owners: (Array.isArray(value) ? value : []) as string[] })
}
</script>

<template>
  <div class="flex flex-col gap-3" data-test="filter-bar">
    <div class="flex flex-wrap items-center gap-3">
      <div class="relative min-w-0 basis-full sm:min-w-56 sm:flex-1 sm:basis-0">
        <FontAwesomeIcon
          :icon="faMagnifyingGlass"
          class="text-muted-foreground pointer-events-none absolute top-1/2 left-3 -translate-y-1/2"
          aria-hidden="true"
        />
        <Input
          ref="search"
          :model-value="filters.query"
          type="search"
          placeholder="Search items, projects and versions"
          aria-label="Search"
          class="bg-surface pr-9 pl-9"
          data-test="search"
          @update:model-value="patch({ query: String($event ?? '') })"
          @keydown.esc="patch({ query: '' })"
        />
        <kbd
          v-if="!filters.query"
          class="border-border text-muted-foreground pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 rounded border px-1.5 font-mono text-[11px]"
          aria-hidden="true"
          >/</kbd
        >
      </div>
      <ToggleGroup
        type="single"
        class="max-w-full overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
        variant="outline"
        size="sm"
        :model-value="filters.severity"
        aria-label="Severity"
        @update:model-value="onSeverity"
      >
        <ToggleGroupItem value="all" data-test="sev-all"
          >All {{ facets.severity.all }}</ToggleGroupItem
        >
        <ToggleGroupItem value="crit" data-test="sev-crit"
          >Critical {{ facets.severity.crit }}</ToggleGroupItem
        >
        <ToggleGroupItem value="warn" data-test="sev-warn"
          >Warnings {{ facets.severity.warn }}</ToggleGroupItem
        >
      </ToggleGroup>
    </div>

    <div class="flex flex-wrap items-center gap-x-5 gap-y-2">
      <ToggleGroup
        type="multiple"
        class="max-w-full overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
        variant="outline"
        size="sm"
        :model-value="filters.kinds"
        aria-label="Kind"
        @update:model-value="onKinds"
      >
        <ToggleGroupItem
          v-for="kind in KIND_GROUPS"
          :key="kind"
          :value="kind"
          :disabled="facets.kinds[kind] === 0 && !filters.kinds.includes(kind)"
          :data-test="`kind-${kind}`"
        >
          <FontAwesomeIcon :icon="KIND_META[kind].icon" aria-hidden="true" />
          {{ KIND_META[kind].label }}
          <span class="text-muted-foreground tabular-nums">{{ facets.kinds[kind] }}</span>
        </ToggleGroupItem>
      </ToggleGroup>
      <ToggleGroup
        v-if="owners.length > 1"
        type="multiple"
        class="max-w-full overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
        variant="outline"
        size="sm"
        :model-value="filters.owners"
        aria-label="Owner"
        @update:model-value="onOwners"
      >
        <ToggleGroupItem
          v-for="owner in owners"
          :key="owner"
          :value="owner"
          :disabled="facets.owners[owner] === 0 && !filters.owners.includes(owner)"
          :data-test="`owner-${owner}`"
        >
          {{ owner }}
          <span class="text-muted-foreground tabular-nums">{{ facets.owners[owner] }}</span>
        </ToggleGroupItem>
      </ToggleGroup>
      <Button
        v-if="active > 0"
        variant="ghost"
        size="sm"
        class="ml-auto"
        data-test="clear-all"
        @click="$emit('clear')"
      >
        <FontAwesomeIcon :icon="faXmark" aria-hidden="true" />
        Clear {{ active }} {{ active === 1 ? 'filter' : 'filters' }}
      </Button>
    </div>
  </div>
</template>
