<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ ratio: number; severity: 'warn' | 'crit' }>()

const SEGMENTS = 10
const filled = computed(() => Math.min(SEGMENTS, Math.max(1, Math.ceil(props.ratio * SEGMENTS))))
</script>

<template>
  <span
    class="inline-flex gap-px"
    role="img"
    :aria-label="`${Math.round(Math.min(ratio, 1) * 100)}% of the critical age`"
  >
    <span
      v-for="i in SEGMENTS"
      :key="i"
      class="h-3 w-1 rounded-[1px]"
      :class="i <= filled ? (severity === 'crit' ? 'bg-crit' : 'bg-warn') : 'bg-border'"
    />
  </span>
</template>
