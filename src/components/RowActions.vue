<script setup lang="ts">
import { ref } from 'vue'
import { faScrewdriverWrench, faSpinner } from '@fortawesome/free-solid-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { actionKey, useTrackerStore } from '@/stores/tracker'
import type { ActionDef, ActionStatus, Row } from '@/lib/types'

const props = defineProps<{ row: Row }>()
const store = useTrackerStore()

// `asking` outlives the dialog so the confirm click still knows which action to run.
const asking = ref<ActionDef | null>(null)
const open = ref(false)

const keyOf = (a: ActionDef) => actionKey(a.id, props.row.repo, props.row.url)
const busy = (a: ActionDef) => store.runningActions.includes(keyOf(a))
const statusOf = (a: ActionDef): ActionStatus | undefined =>
  props.row.actionStatus?.[a.id] ?? store.localStatus[keyOf(a)]

function run(a: ActionDef): void {
  void store.runAction(a.id, props.row.repo, props.row.url)
}

function press(a: ActionDef): void {
  if (!a.confirm) return run(a)
  asking.value = a
  open.value = true
}
</script>

<template>
  <div class="mt-2 flex flex-wrap items-center gap-2" data-test="row-actions">
    <template v-for="a in row.actions" :key="a.id">
      <Button
        size="sm"
        class="font-semibold shadow-sm"
        :disabled="busy(a)"
        :aria-busy="busy(a)"
        :data-test="`action-${a.id}`"
        @click="press(a)"
      >
        <FontAwesomeIcon
          :icon="busy(a) ? faSpinner : faScrewdriverWrench"
          :spin="busy(a)"
          aria-hidden="true"
        />
        {{ a.label }}
      </Button>
      <Badge v-if="busy(a)" variant="secondary" data-test="action-status">Sending</Badge>
      <Badge v-else-if="statusOf(a)" variant="secondary" data-test="action-status">
        <a
          v-if="statusOf(a)?.link"
          :href="statusOf(a)?.link"
          class="underline-offset-4 hover:underline"
          >{{ statusOf(a)?.label || statusOf(a)?.state }}</a
        >
        <template v-else>{{ statusOf(a)?.label || statusOf(a)?.state }}</template>
      </Badge>
    </template>
    <AlertDialog v-model:open="open">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{{ asking?.label }}</AlertDialogTitle>
          <AlertDialogDescription>{{ asking?.confirm }}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction data-test="action-confirm" @click="asking && run(asking)">
            {{ asking?.label }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>
