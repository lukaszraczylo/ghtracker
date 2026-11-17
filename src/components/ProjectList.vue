<script setup lang="ts">
import {
  faArrowsRotate,
  faCircleDot,
  faCodePullRequest,
  faGears,
} from '@fortawesome/free-solid-svg-icons'
import { Card, EmptyState, Heading, Icon, IconButton, Link, StatusDot } from '@perfect-design/ui'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ageSince, isZeroTime } from '@/lib/format'
import type { Repo } from '@/lib/types'

const props = defineProps<{
  repos: Repo[]
  total: number
  focused: string | null
  nowMs: number
  manualRefresh?: boolean
  refreshing?: string[]
  scanning?: boolean
}>()
defineEmits<{ select: [fullName: string]; refresh: [fullName: string] }>()

/** Rows rendered at first and added each time the list is scrolled near its end. */
const PAGE_SIZE = 20

const shown = ref(PAGE_SIZE)
const visible = computed(() => props.repos.slice(0, shown.value))
const hasMore = computed(() => shown.value < props.repos.length)

const scroller = ref<InstanceType<typeof Card> | null>(null)
const scrollEl = () => scroller.value?.$el as HTMLElement | undefined
const sentinel = ref<HTMLElement | null>(null)
let observer: IntersectionObserver | null = null

function loadMore(): void {
  if (hasMore.value) shown.value += PAGE_SIZE
}

// A new search or filter starts the list over from the top.
watch(
  () => props.repos,
  () => {
    shown.value = PAGE_SIZE
    const el = scrollEl()
    if (el) el.scrollTop = 0
  },
)

// Observe the end marker; a fresh marker element is rendered while more rows remain.
watch(
  sentinel,
  (el, prev) => {
    if (prev) observer?.unobserve(prev)
    if (el) observer?.observe(el)
  },
  { flush: 'post' },
)

onMounted(async () => {
  await nextTick()
  if (typeof IntersectionObserver === 'undefined') {
    shown.value = props.repos.length
    return
  }
  observer = new IntersectionObserver(
    (entries) => {
      if (entries.some((e) => e.isIntersecting)) loadMore()
    },
    { root: scrollEl() ?? null, rootMargin: '200px' },
  )
  if (sentinel.value) observer.observe(sentinel.value)
})
onBeforeUnmount(() => observer?.disconnect())

const busy = (name: string) => props.refreshing?.includes(name) ?? false

const TONE = { crit: 'danger', warn: 'warning', ok: 'success' } as const
const LABEL = { crit: 'Critical', warn: 'Warning', ok: 'Healthy' } as const
</script>

<template>
  <section aria-labelledby="projects-title">
    <Heading id="projects-title" :level="2" size="lg" class="mb-4">
      Projects
      <span class="text-muted-foreground text-sm font-normal" data-test="project-count">{{
        repos.length === total ? total : `${repos.length} of ${total}`
      }}</span>
    </Heading>
    <EmptyState
      v-if="repos.length === 0"
      variant="bordered-dashed"
      layout="start"
      size="sm"
      title="No projects match the search."
      title-as="p"
      data-test="no-projects"
    />
    <Card
      v-else
      ref="scroller"
      as="ul"
      size="sm"
      class="divide-border max-h-[70vh] gap-0 divide-y overflow-y-auto p-0 [scrollbar-color:var(--border)_transparent] [scrollbar-width:thin] xl:max-h-[calc(100vh-7rem)]"
    >
      <li
        v-for="repo in visible"
        :key="repo.fullName"
        class="px-3 py-2.5"
        :class="focused === repo.fullName ? 'bg-accent' : ''"
        data-test="project"
      >
        <div class="flex items-center gap-2.5">
          <StatusDot :tone="TONE[repo.health]" size="sm" :label="`${LABEL[repo.health]}:`" />
          <button
            type="button"
            class="hover:text-primary min-w-0 flex-1 truncate text-left text-sm font-medium"
            :aria-pressed="focused === repo.fullName"
            @click="$emit('select', repo.fullName)"
          >
            {{ repo.fullName.split('/')[1] }}
            <span class="text-muted-foreground font-normal">{{ repo.fullName.split('/')[0] }}</span>
          </button>
          <Link
            v-if="repo.release"
            :href="repo.release.url"
            variant="muted"
            size="sm"
            class="font-mono text-xs"
          >
            {{ repo.release.tag }}
          </Link>
          <Link v-else :href="repo.releasesUrl" variant="muted" size="sm" class="text-xs"
            >no release</Link
          >
          <IconButton
            v-if="manualRefresh"
            :icon="faArrowsRotate"
            :label="`Refresh ${repo.fullName}`"
            variant="ghost"
            size="icon-xs"
            class="text-muted-foreground hover:text-primary -my-1 -mr-1"
            :loading="busy(repo.fullName)"
            :disabled="scanning || busy(repo.fullName)"
            data-test="repo-refresh"
            @click="$emit('refresh', repo.fullName)"
          />
        </div>
        <div
          class="text-muted-foreground mt-1 flex items-center gap-4 pl-[18px] text-xs tabular-nums"
        >
          <Link
            :href="`${repo.url}/actions`"
            :variant="repo.workflows.failing ? 'destructive' : 'muted'"
            size="sm"
            class="flex items-center gap-1.5 text-xs no-underline"
          >
            <Icon :icon="faGears" size="xs" />
            {{
              repo.workflows.failing
                ? `${repo.workflows.failing}/${repo.workflows.total} failing`
                : repo.workflows.total
            }}
            <span class="sr-only">workflows</span>
          </Link>
          <Link
            :href="`${repo.url}/pulls`"
            variant="muted"
            size="sm"
            class="flex items-center gap-1.5 text-xs"
          >
            <Icon :icon="faCodePullRequest" size="xs" />{{ repo.prs.open }}
            <span class="sr-only">pull requests</span>
          </Link>
          <Link
            :href="`${repo.url}/issues`"
            variant="muted"
            size="sm"
            class="flex items-center gap-1.5 text-xs"
          >
            <Icon :icon="faCircleDot" size="xs" />{{ repo.issues.open }}
            <span class="sr-only">issues</span>
          </Link>
          <span v-if="repo.release && !isZeroTime(repo.release.publishedAt)" class="ml-auto">
            released {{ ageSince(repo.release.publishedAt, nowMs) }} ago
          </span>
        </div>
      </li>
      <li
        v-if="hasMore"
        ref="sentinel"
        class="text-muted-foreground px-3 py-3 text-center text-xs"
        data-test="more"
      >
        Showing {{ visible.length }} of {{ repos.length }}
      </li>
    </Card>
  </section>
</template>
