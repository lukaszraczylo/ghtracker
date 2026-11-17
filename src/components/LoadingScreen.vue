<script setup lang="ts">
import { faSatelliteDish } from '@fortawesome/free-solid-svg-icons'
import { Card, Heading, Icon, Progress, Skeleton } from '@perfect-design/ui'
import { computed } from 'vue'

const props = defineProps<{ done: number; total: number; connecting: boolean }>()

// Static class names so Tailwind can see every delay; cells cycle through them for a wave effect.
const DELAYS = [
  '[animation-delay:0ms]',
  '[animation-delay:140ms]',
  '[animation-delay:280ms]',
  '[animation-delay:420ms]',
  '[animation-delay:560ms]',
  '[animation-delay:700ms]',
]
const delay = (i: number) => DELAYS[i % DELAYS.length]

const MIN_CELLS = 12
const MAX_CELLS = 63
const cells = computed(() => Math.min(MAX_CELLS, Math.max(MIN_CELLS, props.total)))
// null puts the bar in its indeterminate state until the server reports a total.
const percent = computed(() =>
  !props.connecting && props.total > 0 ? Math.round((props.done / props.total) * 100) : null,
)
const known = computed(() => percent.value !== null)
</script>

<template>
  <section role="status" aria-live="polite" class="mt-10 sm:mt-16" data-test="waiting">
    <div class="flex items-center gap-4">
      <Icon :icon="faSatelliteDish" size="xl" class="text-primary motion-safe:animate-pulse" />
      <div class="min-w-0">
        <Heading :level="1" size="2xl" data-test="loading-title">
          {{ connecting ? 'Connecting' : 'Scanning repositories' }}
        </Heading>
        <p class="text-muted-foreground mt-0.5 text-sm" data-test="loading-detail">
          <template v-if="known">{{ done }} of {{ total }} scanned</template>
          <template v-else-if="connecting">Waiting for the server</template>
          <template v-else>Contacting GitHub</template>
          <span class="hidden sm:inline"> · the first scan can take a minute or two</span>
        </p>
      </div>
    </div>

    <div class="mt-7 max-w-2xl">
      <Progress :model-value="percent" size="sm" label="Scan progress" />
    </div>

    <div class="mt-10" aria-hidden="true">
      <div class="flex items-baseline gap-8">
        <Skeleton class="h-14 w-28 rounded-md sm:h-[4.5rem] sm:w-36" />
        <Skeleton class="h-14 w-28 rounded-md [animation-delay:200ms] sm:h-[4.5rem] sm:w-36" />
      </div>
      <div class="mt-8 grid grid-cols-[repeat(auto-fill,minmax(1.5rem,1fr))] gap-1">
        <Skeleton
          v-for="i in cells"
          :key="i"
          class="h-9 rounded-[3px] sm:h-11"
          :class="delay(i)"
          data-test="skeleton-cell"
        />
      </div>
      <div
        class="border-border mt-7 grid grid-cols-2 gap-x-6 border-t pt-3 sm:grid-cols-3 lg:grid-cols-5"
      >
        <div v-for="i in 5" :key="i" class="py-2">
          <Skeleton class="h-3 w-24 rounded" :class="delay(i)" />
          <Skeleton class="mt-2 h-7 w-12 rounded" :class="delay(i + 1)" />
        </div>
      </div>
    </div>

    <div
      class="mt-10 grid grid-cols-[minmax(0,1fr)] gap-4 lg:grid-cols-[minmax(0,1fr)_24rem]"
      aria-hidden="true"
    >
      <div class="flex flex-col gap-4">
        <Card v-for="g in 3" :key="g" size="sm" class="gap-0 p-4">
          <Skeleton class="h-5 w-48 rounded" :class="delay(g)" />
          <div v-for="r in 2" :key="r" class="mt-4 flex items-center gap-3">
            <Skeleton class="size-8 shrink-0 rounded-md" :class="delay(g + r)" />
            <div class="flex-1 space-y-2">
              <Skeleton class="h-4 w-2/3 rounded" :class="delay(g + r)" />
              <Skeleton class="h-3 w-1/3 rounded" :class="delay(g + r + 1)" />
            </div>
          </div>
        </Card>
      </div>
      <Card size="sm" class="hidden gap-0 p-3 lg:block">
        <div v-for="i in 6" :key="i" class="py-2.5">
          <Skeleton class="h-4 w-3/4 rounded" :class="delay(i)" />
          <Skeleton class="mt-2 h-3 w-1/2 rounded" :class="delay(i + 1)" />
        </div>
      </Card>
    </div>
  </section>
</template>
