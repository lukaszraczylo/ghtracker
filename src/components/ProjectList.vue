<script setup lang="ts">
import { faCodePullRequest, faCircleDot, faGears } from '@fortawesome/free-solid-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import { ageSince, isZeroTime } from '@/lib/format'
import type { Repo } from '@/lib/types'

defineProps<{ repos: Repo[]; total: number; focused: string | null; nowMs: number }>()
defineEmits<{ select: [fullName: string] }>()

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
    <ul v-else class="border-border bg-surface divide-border divide-y rounded-lg border">
      <li
        v-for="repo in repos"
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
    </ul>
  </section>
</template>
