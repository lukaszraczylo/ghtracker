<script setup lang="ts">
import { faSatelliteDish } from '@fortawesome/free-solid-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import { computed } from 'vue'
import { Progress } from '@/components/ui/progress'

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
const percent = computed(() => (props.total > 0 ? Math.round((props.done / props.total) * 100) : 0))
const known = computed(() => !props.connecting && props.total > 0)
</script>

<template>
  <section role="status" aria-live="polite" class="mt-10 sm:mt-16" data-test="waiting">
    <div class="flex items-center gap-4">
      <span
        class="border-border bg-surface text-link relative flex size-12 items-center justify-center rounded-lg border"
      >
        <span
          class="border-link/50 absolute inset-0 rounded-lg border motion-safe:animate-ping"
        ></span>
        <FontAwesomeIcon :icon="faSatelliteDish" class="text-xl" aria-hidden="true" />
      </span>
      <div class="min-w-0">
        <h1 class="text-2xl font-semibold tracking-tight sm:text-3xl" data-test="loading-title">
          {{ connecting ? 'Connecting' : 'Scanning repositories' }}
        </h1>
        <p class="text-muted-foreground mt-0.5 text-sm" data-test="loading-detail">
          <template v-if="known">{{ done }} of {{ total }} scanned</template>
          <template v-else-if="connecting">Waiting for the server</template>
          <template v-else>Contacting GitHub</template>
          <span class="hidden sm:inline"> · the first scan can take a minute or two</span>
        </p>
      </div>
    </div>

    <div class="mt-7 max-w-2xl">
      <Progress v-if="known" :model-value="percent" class="h-1.5" aria-label="Scan progress" />
      <div v-else class="bg-secondary h-1.5 w-full overflow-hidden rounded-full">
        <div class="bg-primary h-full w-1/3 rounded-full motion-safe:animate-pulse"></div>
      </div>
    </div>

    <div class="mt-10" aria-hidden="true">
      <div class="flex items-baseline gap-8">
        <div
          class="bg-secondary h-14 w-28 rounded-md motion-safe:animate-pulse sm:h-[4.5rem] sm:w-36"
        ></div>
        <div
          class="bg-secondary h-14 w-28 rounded-md [animation-delay:200ms] motion-safe:animate-pulse sm:h-[4.5rem] sm:w-36"
        ></div>
      </div>
      <div class="mt-8 grid grid-cols-[repeat(auto-fill,minmax(1.5rem,1fr))] gap-1">
        <div
          v-for="i in cells"
          :key="i"
          class="bg-secondary h-9 rounded-[3px] motion-safe:animate-pulse sm:h-11"
          :class="delay(i)"
        ></div>
      </div>
      <div
        class="border-border mt-7 grid grid-cols-2 gap-x-6 border-t pt-3 sm:grid-cols-3 lg:grid-cols-5"
      >
        <div v-for="i in 5" :key="i" class="py-2">
          <div
            class="bg-secondary h-3 w-24 rounded motion-safe:animate-pulse"
            :class="delay(i)"
          ></div>
          <div
            class="bg-secondary mt-2 h-7 w-12 rounded motion-safe:animate-pulse"
            :class="delay(i + 1)"
          ></div>
        </div>
      </div>
    </div>

    <div
      class="mt-10 grid grid-cols-[minmax(0,1fr)] gap-4 lg:grid-cols-[minmax(0,1fr)_24rem]"
      aria-hidden="true"
    >
      <div class="flex flex-col gap-4">
        <div v-for="g in 3" :key="g" class="border-border bg-surface rounded-lg border p-4">
          <div
            class="bg-secondary h-5 w-48 rounded motion-safe:animate-pulse"
            :class="delay(g)"
          ></div>
          <div v-for="r in 2" :key="r" class="mt-4 flex items-center gap-3">
            <div
              class="bg-secondary size-8 shrink-0 rounded-md motion-safe:animate-pulse"
              :class="delay(g + r)"
            ></div>
            <div class="flex-1 space-y-2">
              <div
                class="bg-secondary h-4 w-2/3 rounded motion-safe:animate-pulse"
                :class="delay(g + r)"
              ></div>
              <div
                class="bg-secondary h-3 w-1/3 rounded motion-safe:animate-pulse"
                :class="delay(g + r + 1)"
              ></div>
            </div>
          </div>
        </div>
      </div>
      <div class="border-border bg-surface hidden rounded-lg border p-3 lg:block">
        <div v-for="i in 6" :key="i" class="py-2.5">
          <div
            class="bg-secondary h-4 w-3/4 rounded motion-safe:animate-pulse"
            :class="delay(i)"
          ></div>
          <div
            class="bg-secondary mt-2 h-3 w-1/2 rounded motion-safe:animate-pulse"
            :class="delay(i + 1)"
          ></div>
        </div>
      </div>
    </div>
  </section>
</template>
