<script setup lang="ts">
import { faArrowsRotate } from '@fortawesome/free-solid-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import { Button } from '@/components/ui/button'

defineProps<{ repo: string; busy: boolean; blocked: boolean }>()
defineEmits<{ refresh: [] }>()
</script>

<template>
  <Button
    variant="ghost"
    size="icon-xs"
    class="text-muted-foreground hover:text-link shrink-0"
    :disabled="busy || blocked"
    :aria-label="`Refresh ${repo}`"
    :aria-busy="busy"
    :title="`Refresh ${repo}`"
    data-test="repo-refresh"
    @click="$emit('refresh')"
  >
    <FontAwesomeIcon :icon="faArrowsRotate" :spin="busy" aria-hidden="true" />
  </Button>
</template>
