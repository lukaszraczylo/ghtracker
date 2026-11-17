<script setup lang="ts">
import { faArrowsRotate, faSatelliteDish } from '@fortawesome/free-solid-svg-icons'
import { AppHeader, Button, Icon, ThemeToggle } from '@perfect-design/ui'
import { computed } from 'vue'
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
// The header row cannot wrap, so phones show this line under it instead.
const status = computed(() =>
  updated.value ? `Scanned ${updated.value} ago · next in ${nextIn.value}` : 'Loading',
)
</script>

<template>
  <AppHeader :bordered="false" class="-mx-4 w-auto">
    <template #brand>
      <Icon :icon="faSatelliteDish" size="lg" class="text-primary" />
      <span class="text-lg font-semibold tracking-tight">ghtracker</span>
    </template>
    <template #actions>
      <span class="text-muted-foreground hidden text-sm sm:inline" data-test="updated">{{
        status
      }}</span>
      <Button
        v-if="manualRefresh"
        variant="outline"
        size="sm"
        :disabled="scanning"
        data-test="rescan"
        @click="$emit('rescan')"
      >
        <template #icon-start><Icon :icon="faArrowsRotate" :spin="scanning" /></template>
        <span class="hidden sm:inline">{{
          scanning ? `Scanning ${progress}`.trim() : 'Rescan repositories'
        }}</span>
        <span class="sm:hidden">{{ scanning ? progress || 'Scanning…' : 'Rescan' }}</span>
      </Button>
      <ThemeToggle size="sm" />
    </template>
  </AppHeader>
  <p class="text-muted-foreground text-xs sm:hidden">{{ status }}</p>
</template>
