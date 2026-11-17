<script setup lang="ts">
import {
  faCodePullRequest,
  faCircleDot,
  faGears,
  faTriangleExclamation,
  type IconDefinition,
} from '@fortawesome/free-solid-svg-icons'
import { Badge, Heading, KpiTile, StatGroup, type Tone } from '@perfect-design/ui'
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

const rows = computed<{ label: string; value: number; icon: IconDefinition; tone: Tone }[]>(() => [
  {
    label: 'Workflows failing',
    value: props.facts.failingWorkflows,
    icon: faGears,
    tone: 'danger',
  },
  {
    label: 'Pull requests waiting',
    value: props.facts.waitingPrs,
    icon: faCodePullRequest,
    tone: 'danger',
  },
  {
    label: 'PRs with failing checks',
    value: props.facts.failingChecks,
    icon: faCodePullRequest,
    tone: 'warning',
  },
  { label: 'Stale issues', value: props.facts.staleIssues, icon: faCircleDot, tone: 'warning' },
  {
    label: 'Data problems',
    value: props.facts.dataProblems,
    icon: faTriangleExclamation,
    tone: 'warning',
  },
])
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
      <Heading
        :level="1"
        class="flex flex-wrap items-baseline gap-x-6 gap-y-1 sm:gap-x-10"
        data-test="headline"
      >
        <template v-if="crit > 0 || warn > 0">
          <span v-if="crit > 0" class="text-destructive">
            <span class="text-5xl leading-none tabular-nums sm:text-7xl">{{ crit }}</span>
            <span class="ml-2 text-xl font-medium">critical</span>
          </span>
          <span v-if="warn > 0" class="text-warning">
            <span class="text-5xl leading-none tabular-nums sm:text-7xl">{{ warn }}</span>
            <span class="ml-2 text-xl font-medium">{{ warn === 1 ? 'warning' : 'warnings' }}</span>
          </span>
        </template>
        <span v-else class="text-success text-5xl leading-none sm:text-7xl">All clear</span>
      </Heading>
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
        <Badge dot tone="danger" variant="outline" size="sm">critical</Badge>
        <Badge dot tone="warning" variant="outline" size="sm">warning</Badge>
        <Badge dot tone="success" variant="outline" size="sm">healthy</Badge>
        <span class="sm:ml-auto">Tap a segment to focus a project</span>
      </p>
    </div>

    <StatGroup :columns="5" layout="divided" data-test="facts">
      <KpiTile
        v-for="row in rows"
        :key="row.label"
        :label="row.label"
        :value="row.value"
        :icon="row.icon"
        :tone="row.value > 0 ? row.tone : 'neutral'"
        size="compact"
        data-test="fact"
      />
    </StatGroup>
  </section>
</template>
