<script setup lang="ts">
import { faArrowsRotate, faTag } from '@fortawesome/free-solid-svg-icons'
import {
  Badge,
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
  Icon,
  IconButton,
  Link,
} from '@perfect-design/ui'
import AttentionRow from '@/components/AttentionRow.vue'
import type { Block } from '@/lib/types'

const props = defineProps<{
  block: Block
  nowMs: number
  critSeconds: number
  manualRefresh?: boolean
  refreshing?: string[]
  scanning?: boolean
}>()
defineEmits<{ refresh: [fullName: string] }>()

const busy = () => props.refreshing?.includes(props.block.repo.fullName) ?? false
</script>

<template>
  <Card as="section" size="sm" class="gap-0 p-0" data-test="repo-block">
    <CardHeader
      as="header"
      class="border-border flex flex-wrap items-center gap-x-3 gap-y-1 border-b px-4 py-3"
    >
      <CardTitle as="h3">
        <Link :href="block.repo.url" variant="subtle">
          <span class="text-muted-foreground font-normal"
            >{{ block.repo.fullName.split('/')[0] }}/</span
          >{{ block.repo.fullName.split('/')[1] }}
        </Link>
      </CardTitle>
      <Badge v-if="block.repo.release" variant="secondary" as-child class="font-mono">
        <a :href="block.repo.release.url">
          <Icon :icon="faTag" size="xs" />
          {{ block.repo.release.tag }}
        </a>
      </Badge>
      <CardAction class="text-muted-foreground ml-auto flex items-center gap-1 text-xs">
        {{ block.rows.length }} {{ block.rows.length === 1 ? 'item' : 'items' }}
        <IconButton
          v-if="manualRefresh"
          :icon="faArrowsRotate"
          :label="`Refresh ${block.repo.fullName}`"
          variant="ghost"
          size="icon-xs"
          class="text-muted-foreground hover:text-primary"
          :loading="busy()"
          :disabled="scanning || busy()"
          data-test="repo-refresh"
          @click="$emit('refresh', block.repo.fullName)"
        />
      </CardAction>
    </CardHeader>
    <CardContent as="ul" class="divide-border divide-y px-4 py-0">
      <AttentionRow
        v-for="row in block.rows"
        :key="row.url"
        :row="row"
        :now-ms="nowMs"
        :crit-seconds="critSeconds"
      />
    </CardContent>
  </Card>
</template>
