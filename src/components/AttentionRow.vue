<script setup lang="ts">
import { computed } from 'vue'
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import AgeMeter from '@/components/AgeMeter.vue'
import { ageSince, isZeroTime, secondsSince, shorten } from '@/lib/format'
import { iconFor } from '@/lib/kinds'
import type { Row } from '@/lib/types'

const props = defineProps<{ row: Row; nowMs: number; critSeconds: number }>()

const ratio = computed(() => secondsSince(props.row.since, props.nowMs) / props.critSeconds)
</script>

<template>
  <li
    class="grid grid-cols-[2rem_minmax(0,1fr)] items-start gap-x-3 gap-y-1 py-3 sm:grid-cols-[2rem_minmax(0,1fr)_auto]"
    data-test="row"
  >
    <span
      class="flex size-8 items-center justify-center rounded-md"
      :class="row.severity === 'crit' ? 'bg-crit/12 text-crit' : 'bg-warn/12 text-warn'"
    >
      <FontAwesomeIcon :icon="iconFor(row.kind)" aria-hidden="true" />
      <span class="sr-only">{{ row.severity === 'crit' ? 'Critical' : 'Warning' }}</span>
    </span>
    <div class="min-w-0">
      <a
        :href="row.url"
        class="hover:text-link leading-snug font-medium wrap-anywhere underline-offset-4 hover:underline"
      >
        {{ shorten(row.subject) }}
      </a>
      <p class="text-muted-foreground mt-0.5 text-sm wrap-anywhere">
        {{ shorten(row.detail, 120) }}
      </p>
    </div>
    <div
      class="col-start-2 flex items-center gap-3 sm:col-start-auto sm:flex-col sm:items-end sm:gap-1.5 sm:pt-0.5"
    >
      <span class="text-muted-foreground font-mono text-xs whitespace-nowrap">
        {{ isZeroTime(row.since) ? '' : ageSince(row.since, nowMs) }}
      </span>
      <AgeMeter
        v-if="row.kind === 'pr_waiting' && !isZeroTime(row.since)"
        :ratio="ratio"
        :severity="row.severity"
      />
    </div>
  </li>
</template>
