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
  <section class="flex flex-col gap-7 sm:gap-8" aria-label="Fleet status">
    <div>
      <p class="text-muted-foreground mb-3 text-sm" data-test="scope">
        <template v-if="blockCount > 0"
          >{{ blockCount }} of {{ repos.length }} projects need attention</template
        >
        <template v-else>All {{ repos.length }} projects are healthy</template>
      </p>
      <h1
        class="flex flex-wrap items-baseline gap-x-6 gap-y-1 font-semibold tracking-tight sm:gap-x-10"
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
    </div>

    <div>
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

    <dl
      class="border-border grid grid-cols-2 gap-x-6 border-t sm:grid-cols-3 lg:grid-cols-5"
      data-test="facts"
    >
      <div
        v-for="row in rows"
        :key="row.label"
        class="border-border border-b py-3 last:col-span-2 sm:last:col-span-1 lg:border-b-0"
      >
        <dt class="text-muted-foreground flex items-center gap-2 text-xs">
          <FontAwesomeIcon :icon="row.icon" class="w-3.5" aria-hidden="true" />
          {{ row.label }}
        </dt>
        <dd
          class="mt-1 font-mono text-3xl tabular-nums"
          :class="row.value > 0 ? row.tone : 'text-muted-foreground'"
        >
          {{ row.value }}
        </dd>
      </div>
    </dl>
  </section>
</template>
