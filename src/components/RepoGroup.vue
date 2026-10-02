<script setup lang="ts">
import { faTag } from '@fortawesome/free-solid-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import AttentionRow from '@/components/AttentionRow.vue'
import { Badge } from '@/components/ui/badge'
import type { Block } from '@/lib/types'

defineProps<{ block: Block; nowMs: number; critSeconds: number }>()
</script>

<template>
  <section class="border-border bg-surface rounded-lg border" data-test="repo-block">
    <header class="border-border flex flex-wrap items-center gap-x-3 gap-y-1 border-b px-4 py-3">
      <h3 class="font-semibold">
        <a :href="block.repo.url" class="hover:text-link underline-offset-4 hover:underline">
          <span class="text-muted-foreground font-normal"
            >{{ block.repo.fullName.split('/')[0] }}/</span
          >{{ block.repo.fullName.split('/')[1] }}
        </a>
      </h3>
      <Badge v-if="block.repo.release" variant="secondary" as-child class="font-mono">
        <a :href="block.repo.release.url">
          <FontAwesomeIcon :icon="faTag" aria-hidden="true" class="text-[0.7em]" />
          {{ block.repo.release.tag }}
        </a>
      </Badge>
      <span class="text-muted-foreground ml-auto text-xs">
        {{ block.rows.length }} {{ block.rows.length === 1 ? 'item' : 'items' }}
      </span>
    </header>
    <ul class="divide-border divide-y px-4">
      <AttentionRow
        v-for="row in block.rows"
        :key="row.url"
        :row="row"
        :now-ms="nowMs"
        :crit-seconds="critSeconds"
      />
    </ul>
  </section>
</template>
