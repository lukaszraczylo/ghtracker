<script setup lang="ts">
import {
  faCodePullRequest,
  faCircleDot,
  faGears,
  faTriangleExclamation,
  type IconDefinition,
} from '@fortawesome/free-solid-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import { computed } from 'vue'
import FleetStrip from '@/components/FleetStrip.vue'
import type { Repo } from '@/lib/types'
import type { Facts, RepoCounts } from '@/stores/tracker'

const props = defineProps<{
  crit: number
  warn: number
  blockCount: number
  repos: Repo[]
  counts: Map<string, RepoCounts>
  facts: Facts
  focused: string | null
  matching: Set<string>
}>()
defineEmits<{ select: [fullName: string] }>()

const rows = computed<{ label: string; value: number; icon: IconDefinition; tone: string }[]>(
  () => [
    {
      label: 'Workflows failing',
      value: props.facts.failingWorkflows,
      icon: faGears,
      tone: 'text-crit',
    },
    {
      label: 'Pull requests waiting',
      value: props.facts.waitingPrs,
      icon: faCodePullRequest,
      tone: 'text-crit',
    },
    {
      label: 'PRs with failing checks',
      value: props.facts.failingChecks,
      icon: faCodePullRequest,
      tone: 'text-warn',
    },
    { label: 'Stale issues', value: props.facts.staleIssues, icon: faCircleDot, tone: 'text-warn' },
    {
      label: 'Data problems',
      value: props.facts.dataProblems,
      icon: faTriangleExclamation,
      tone: 'text-warn',
    },
  ],
)
</script>

<template>
  <section
    class="grid grid-cols-[minmax(0,1fr)] items-end gap-x-14 gap-y-8 lg:grid-cols-[minmax(0,1fr)_22rem]"
    aria-label="Fleet status"
  >
    <div>
      <p class="text-muted-foreground mb-3 text-sm" data-test="scope">
        <template v-if="blockCount > 0"
          >{{ blockCount }} of {{ repos.length }} projects need attention</template
        >
        <template v-else>All {{ repos.length }} projects are healthy</template>
      </p>
      <h1
        class="flex flex-wrap items-baseline gap-x-6 gap-y-1 sm:gap-x-10 font-semibold tracking-tight"
        data-test="headline"
      >
        <template v-if="crit > 0 || warn > 0">
          <span v-if="crit > 0" class="text-crit">
            <span class="text-5xl leading-none tabular-nums sm:text-7xl">{{ crit }}</span>
            <span class="ml-2 text-xl font-medium">critical</span>
          </span>
          <span v-if="warn > 0" class="text-warn">
            <span class="text-5xl leading-none tabular-nums sm:text-7xl">{{ warn }}</span>
            <span class="ml-2 text-xl font-medium">{{ warn === 1 ? 'warning' : 'warnings' }}</span>
          </span>
        </template>
        <span v-else class="text-ok text-5xl leading-none sm:text-7xl">All clear</span>
      </h1>
      <div class="mt-6 sm:mt-8">
        <FleetStrip
          :repos="repos"
          :counts="counts"
          :focused="focused"
          :matching="matching"
          @select="$emit('select', $event)"
        />
        <p class="text-muted-foreground mt-3 flex flex-wrap items-center gap-x-5 gap-y-1 text-xs">
          <span class="flex items-center gap-1.5"
            ><span class="bg-crit size-2 rounded-[2px]"></span>critical</span
          >
          <span class="flex items-center gap-1.5"
            ><span class="bg-warn size-2 rounded-[2px]"></span>warning</span
          >
          <span class="flex items-center gap-1.5"
            ><span class="bg-ok/40 size-2 rounded-[2px]"></span>healthy</span
          >
          <span class="sm:ml-auto">Tap a segment to focus a project</span>
        </p>
      </div>
    </div>

    <dl class="border-border divide-border divide-y border-y text-sm" data-test="facts">
      <div v-for="row in rows" :key="row.label" class="flex items-center gap-3 py-2.5">
        <FontAwesomeIcon :icon="row.icon" class="text-muted-foreground w-4" aria-hidden="true" />
        <dt class="flex-1">{{ row.label }}</dt>
        <dd
          class="font-mono text-base tabular-nums"
          :class="row.value > 0 ? row.tone : 'text-muted-foreground'"
        >
          {{ row.value }}
        </dd>
      </div>
    </dl>
  </section>
</template>
