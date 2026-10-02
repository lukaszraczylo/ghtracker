import {
  faClockRotateLeft,
  faCodePullRequest,
  faCircleDot,
  faGears,
  faPlugCircleXmark,
  faTriangleExclamation,
  type IconDefinition,
} from '@fortawesome/free-solid-svg-icons'

/** Maps a server alert kind to the icon and group used in the UI. */
export const KIND_ICON: Record<string, IconDefinition> = {
  workflow_failed: faGears,
  workflow_stuck: faGears,
  pr_waiting: faCodePullRequest,
  pr_checks_failing: faCodePullRequest,
  issues_stale: faCircleDot,
  refresh_failed: faPlugCircleXmark,
  data_stale: faClockRotateLeft,
  partial_data: faTriangleExclamation,
}

export function iconFor(kind: string): IconDefinition {
  return KIND_ICON[kind] ?? faTriangleExclamation
}

export const DATA_KINDS = ['refresh_failed', 'data_stale', 'partial_data']
