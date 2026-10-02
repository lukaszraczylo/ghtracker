<script setup lang="ts">
import { faArrowsRotate, faSatelliteDish } from '@fortawesome/free-solid-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import { computed } from 'vue'
import { Button } from '@/components/ui/button'
import { ageLabel, ageSince, isZeroTime } from '@/lib/format'

const props = defineProps<{
  lastRefresh: string
  nowMs: number
  intervalSeconds: number
  loaded: boolean
  scanning: boolean
  progress: string
  manualRefresh: boolean
}>()
defineEmits<{ rescan: [] }>()

const updated = computed(() =>
  props.loaded && !isZeroTime(props.lastRefresh) ? ageSince(props.lastRefresh, props.nowMs) : '',
)
const nextIn = computed(() => {
  if (!updated.value) return ''
  const left = Math.floor(
    (Date.parse(props.lastRefresh) + props.intervalSeconds * 1000 - props.nowMs) / 1000,
  )
  return left > 0 ? ageLabel(left) : 'now'
})
</script>

<template>
  <header class="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
    <div class="flex items-center gap-3">
      <span
        class="border-border bg-surface text-link flex size-9 items-center justify-center rounded-md border"
      >
        <FontAwesomeIcon :icon="faSatelliteDish" aria-hidden="true" />
      </span>
      <span class="text-lg font-semibold tracking-tight">ghtracker</span>
    </div>
    <div class="text-muted-foreground flex flex-wrap items-center gap-x-5 gap-y-1 text-sm">
      <span
        v-if="updated"
        class="order-last w-full text-xs sm:order-none sm:w-auto sm:text-sm"
        data-test="updated"
      >
        Scanned {{ updated }} ago
        <span class="text-muted-foreground/70"> · next in {{ nextIn }}</span>
      </span>
      <span v-else>Loading</span>
      <Button
        v-if="manualRefresh"
        variant="outline"
        size="sm"
        :disabled="scanning"
        data-test="rescan"
        @click="$emit('rescan')"
      >
        <FontAwesomeIcon :icon="faArrowsRotate" :spin="scanning" aria-hidden="true" />
        <span class="hidden sm:inline">{{
          scanning ? `Scanning ${progress}`.trim() : 'Rescan repositories'
        }}</span>
        <span class="sm:hidden">{{ scanning ? progress || 'Scanning…' : 'Rescan' }}</span>
      </Button>
    </div>
  </header>
</template>
