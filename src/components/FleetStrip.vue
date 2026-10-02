<script setup lang="ts">
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import type { Repo } from '@/lib/types'
import type { RepoCounts } from '@/stores/tracker'

defineProps<{
  repos: Repo[]
  counts: Map<string, RepoCounts>
  focused: string | null
  matching: Set<string>
}>()
defineEmits<{ select: [fullName: string] }>()

const SEGMENT = { crit: 'bg-crit', warn: 'bg-warn', ok: 'bg-ok/30 hover:bg-ok/60' } as const

function summary(c: RepoCounts | undefined): string {
  if (!c) return 'healthy'
  const parts = []
  if (c.crit) parts.push(`${c.crit} critical`)
  if (c.warn) parts.push(`${c.warn} ${c.warn === 1 ? 'warning' : 'warnings'}`)
  return parts.join(' · ')
}
</script>

<template>
  <TooltipProvider :delay-duration="80">
    <div
      class="grid grid-cols-[repeat(auto-fill,minmax(1.5rem,1fr))] gap-1"
      role="group"
      aria-label="Project health"
      data-test="strip"
    >
      <Tooltip v-for="repo in repos" :key="repo.fullName">
        <TooltipTrigger as-child>
          <button
            type="button"
            :aria-label="`${repo.fullName}: ${summary(counts.get(repo.fullName))}`"
            :aria-pressed="focused === repo.fullName"
            class="h-9 min-w-0 rounded-[3px] sm:h-11 transition-colors focus-visible:outline-offset-1"
            :class="[
              SEGMENT[repo.health],
              focused === repo.fullName
                ? 'ring-foreground ring-2 ring-offset-2 ring-offset-background'
                : '',
            ]"
            data-test="segment"
            @click="$emit('select', repo.fullName)"
          />
        </TooltipTrigger>
        <TooltipContent>
          <p class="font-medium">{{ repo.fullName }}</p>
          <p class="opacity-80">{{ summary(counts.get(repo.fullName)) }}</p>
        </TooltipContent>
      </Tooltip>
    </div>
  </TooltipProvider>
</template>
