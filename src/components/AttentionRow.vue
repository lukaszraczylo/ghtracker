<script setup lang="ts">
import { Icon, Link, Progress } from '@perfect-design/ui'
import { computed } from 'vue'
import RowActions from '@/components/RowActions.vue'
import { ageSince, isZeroTime, secondsSince, shorten } from '@/lib/format'
import { iconFor } from '@/lib/kinds'
import type { Row } from '@/lib/types'

const props = defineProps<{ row: Row; nowMs: number; critSeconds: number }>()

const AGE_SEGMENTS = 10
const ratio = computed(() => secondsSince(props.row.since, props.nowMs) / props.critSeconds)
// Whole segments, at least one, so a fresh PR still shows on the meter.
const ageFill = computed(
  () => Math.min(AGE_SEGMENTS, Math.max(1, Math.ceil(ratio.value * AGE_SEGMENTS))) * AGE_SEGMENTS,
)
const crit = computed(() => props.row.severity === 'crit')
</script>

<template>
  <li
    class="grid grid-cols-[2rem_minmax(0,1fr)] items-start gap-x-3 gap-y-1 py-3 sm:grid-cols-[2rem_minmax(0,1fr)_auto]"
    data-test="row"
  >
    <Icon
      :icon="iconFor(row.kind)"
      size="lg"
      :label="crit ? 'Critical' : 'Warning'"
      class="mt-0.5 justify-self-center"
      :class="crit ? 'text-destructive' : 'text-warning'"
    />
    <div class="min-w-0">
      <Link :href="row.url" variant="subtle" class="leading-snug font-medium wrap-anywhere">
        {{ shorten(row.subject) }}
      </Link>
      <p class="text-muted-foreground mt-0.5 text-sm wrap-anywhere">
        {{ shorten(row.detail, 120) }}
      </p>
      <RowActions v-if="row.actions?.length" :row="row" />
    </div>
    <div
      class="col-start-2 flex items-center gap-3 sm:col-start-auto sm:flex-col sm:items-end sm:gap-1.5 sm:pt-0.5"
    >
      <span class="text-muted-foreground font-mono text-xs whitespace-nowrap">
        {{ isZeroTime(row.since) ? '' : ageSince(row.since, nowMs) }}
      </span>
      <Progress
        v-if="row.kind === 'pr_waiting' && !isZeroTime(row.since)"
        :model-value="ageFill"
        :segments="AGE_SEGMENTS"
        :tone="crit ? 'danger' : 'warning'"
        size="md"
        :label="`${Math.round(Math.min(ratio, 1) * 100)}% of the critical age`"
        class="w-20"
        data-test="age-meter"
      />
    </div>
  </li>
</template>
