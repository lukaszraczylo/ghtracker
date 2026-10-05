<script setup lang="ts">
import { faCodePullRequest, faCircleDot, faGears } from '@fortawesome/free-solid-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import RepoRefreshButton from '@/components/RepoRefreshButton.vue'
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

const scroller = ref<HTMLElement | null>(null)
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
    if (scroller.value) scroller.value.scrollTop = 0
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
    { root: scroller.value, rootMargin: '200px' },
  )
  if (sentinel.value) observer.observe(sentinel.value)
})
onBeforeUnmount(() => observer?.disconnect())

const DOT = { crit: 'bg-crit', warn: 'bg-warn', ok: 'bg-ok' } as const
const LABEL = { crit: 'Critical', warn: 'Warning', ok: 'Healthy' } as const
</script>

<template>
  <section aria-labelledby="projects-title">
    <h2 id="projects-title" class="mb-4 text-lg font-semibold tracking-tight">
      Projects
      <span class="text-muted-foreground text-sm font-normal" data-test="project-count">{{
        repos.length === total ? total : `${repos.length} of ${total}`
      }}</span>
    </h2>
    <p
      v-if="repos.length === 0"
      class="text-muted-foreground border-border rounded-lg border border-dashed p-4 text-sm"
      data-test="no-projects"
    >
      No projects match the search.
    </p>
    <ul
      v-else
      ref="scroller"
      class="border-border bg-surface divide-border max-h-[70vh] divide-y overflow-y-auto rounded-lg border [scrollbar-color:var(--border)_transparent] [scrollbar-width:thin] xl:max-h-[calc(100vh-7rem)]"
    >
      <li
        v-for="repo in visible"
        :key="repo.fullName"
        class="px-3 py-2.5"
        :class="focused === repo.fullName ? 'bg-accent' : ''"
        data-test="project"
      >
        <div class="flex items-center gap-2.5">
          <span class="size-2 shrink-0 rounded-full" :class="DOT[repo.health]"></span>
          <span class="sr-only">{{ LABEL[repo.health] }}:</span>
          <button
            type="button"
            class="hover:text-link min-w-0 flex-1 truncate text-left text-sm font-medium"
            :aria-pressed="focused === repo.fullName"
            @click="$emit('select', repo.fullName)"
          >
            {{ repo.fullName.split('/')[1] }}
            <span class="text-muted-foreground font-normal">{{ repo.fullName.split('/')[0] }}</span>
          </button>
          <a
            v-if="repo.release"
            :href="repo.release.url"
            class="text-muted-foreground hover:text-link font-mono text-xs"
          >
            {{ repo.release.tag }}
          </a>
          <a v-else :href="repo.releasesUrl" class="text-muted-foreground hover:text-link text-xs"
            >no release</a
          >
          <RepoRefreshButton
            v-if="manualRefresh"
            :repo="repo.fullName"
            :busy="refreshing?.includes(repo.fullName) ?? false"
            :blocked="scanning ?? false"
            class="-my-1 -mr-1"
            @refresh="$emit('refresh', repo.fullName)"
          />
        </div>
        <div
          class="text-muted-foreground mt-1 flex items-center gap-4 pl-[18px] text-xs tabular-nums"
        >
          <a
            :href="`${repo.url}/actions`"
            class="hover:text-link flex items-center gap-1.5"
            :class="repo.workflows.failing ? 'text-crit' : ''"
          >
            <FontAwesomeIcon :icon="faGears" aria-hidden="true" />
            {{
              repo.workflows.failing
                ? `${repo.workflows.failing}/${repo.workflows.total} failing`
                : repo.workflows.total
            }}
            <span class="sr-only">workflows</span>
          </a>
          <a :href="`${repo.url}/pulls`" class="hover:text-link flex items-center gap-1.5">
            <FontAwesomeIcon :icon="faCodePullRequest" aria-hidden="true" />{{ repo.prs.open }}
            <span class="sr-only">pull requests</span>
          </a>
          <a :href="`${repo.url}/issues`" class="hover:text-link flex items-center gap-1.5">
            <FontAwesomeIcon :icon="faCircleDot" aria-hidden="true" />{{ repo.issues.open }}
            <span class="sr-only">issues</span>
          </a>
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
    </ul>
  </section>
</template>
