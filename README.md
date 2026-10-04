# ghtracker

Dashboard and Prometheus metrics for a list of GitHub repositories. The program authenticates as a GitHub App and refreshes every 6 hours by default.

## Install

Each release publishes binaries for Linux, macOS and Windows on the [releases page](https://github.com/lukaszraczylo/ghtracker/releases), and a multi-architecture container image:

```sh
docker pull ghcr.io/lukaszraczylo/ghtracker:latest
```

Release archives and the image checksums are signed with cosign.

## What it shows

The dashboard lists only what needs attention, grouped by project. The project with the newest problem comes first:

- Failing workflows, including scheduled and other background ones, with a link to the run.
- Pull requests that wait too long or fail their checks, with links and age.
- Stale issues, as one line per project.
- Projects that failed to refresh, or loaded only in part.

A side panel lists every project with its latest release (or newest tag), a link to the release, and counts of workflows, open pull requests and issues. Healthy items do not appear anywhere else.

### Search and filters

- Type in the search box, or press `/`, to match projects, versions, item titles and details. Every word must match. Press `Esc` to clear.
- Filter by severity, by kind (workflows, pull requests, issues, data problems) and by owner. Each chip shows how many items it would leave, given the other filters, and chips that would match nothing are disabled.
- Select a segment of the health strip or a project name to focus one project.
- The URL holds the filters (`?q=bump&kind=pr&owner=lukaszraczylo`), so you can bookmark or share a filtered view.

## Setup

1. Create a GitHub App with read-only repository permissions: Metadata, Issues, Pull requests, Actions, Checks, Commit statuses and Contents.
2. Install the App on the repositories you want to observe.
3. Generate a private key for the App.
4. Copy `config.example.yaml` to `config.yaml`, then set `github.app_id` and the key path. You can set `GHTRACKER_PRIVATE_KEY` to the PEM contents instead of a file.
5. Run `make build`, then `./ghtracker -config config.yaml`, and open http://localhost:8080.

## Alert rules

| Condition                                                                                | Severity |
| ---------------------------------------------------------------------------------------- | -------- |
| Latest completed default-branch run of a workflow failed (includes background workflows) | crit     |
| Non-draft PR open longer than `pr_crit_after`                                            | crit     |
| Repo refresh failed (the dashboard keeps the last good data)                             | crit     |
| Non-draft PR open longer than `pr_warn_after`                                            | warn     |
| PR checks failing                                                                        | warn     |
| Workflow queued or running longer than `workflow_stuck_after`                            | warn     |
| Open issues without activity for `issue_stale_after` (one alert per repo)                | warn     |
| A section failed to load, for example a missing App permission                           | warn     |
| Data older than `data_stale_after`                                                       | warn     |

Alerts are evaluated when you read them, so PR ages keep advancing between refreshes. Set a threshold to a negative value to disable it. Draft PRs and archived repos produce no work alerts.

## Endpoints

- `/` dashboard.
- `/metrics` Prometheus metrics.
- `/api/state` full state as JSON.
- `/healthz` returns 503 until the first refresh completes.
- `POST /refresh` runs an extra refresh and backs the Refresh button. It is on by default, limited to once per 5 minutes, and refuses cross-site requests. Set `manual_refresh: false` to disable it.
- `POST /refresh?repo=owner/name` refreshes only that repository and answers when it finishes. An empty `repo` value runs the full refresh above.
  - The repository must be in the configured list, or the answer is 404 with a JSON `{"error": ...}` body. The match ignores case.
  - On success the answer is 200 with the same JSON as `/api/state`, plus a `repo` field. Only that repository changes. The last scan time and the schedule do not move.
  - Each repository has its own cooldown of 60 seconds. Inside it, the answer is 429 with a `Retry-After` header. A repository refresh does not use the 5-minute budget of the full refresh, and the reverse also holds.
  - While a full refresh or a refresh of the same repository runs, the answer is 409. The cooldown does not start, so you can retry at once.
  - The same cross-site refusal and `manual_refresh: false` switch apply. The GitHub rate-limit protections below apply too.

The dashboard shows a refresh button beside each project when `manual_refresh` is on.

## Alert actions

An action adds a button to alerts. Pressing it sends the alert as JSON to a webhook that you run, for example to re-run a failed pipeline or to open a ticket. Without an `actions` block the feature is off: the state document, the endpoints and the UI do not change.

```yaml
actions:
  - id: rerun # [a-z0-9-]+, appears in the payload and the endpoint
    label: Re-run # button text
    kinds: [workflow_failed] # alert kinds that show the button; omit for every kind
    webhook:
      url: https://example.internal/hooks/rerun
      token_env: ACTION_TOKEN # name of an environment variable that holds a bearer token; optional
      timeout: 10s
    status_url: https://example.internal/hooks/rerun/status # optional
    confirm: "Run this action for the selected alert?" # optional; omit for no dialog
```

- `POST /api/actions/{id}` takes `{"repo": "owner/name", "url": "<alert url>"}`. It refuses cross-site requests like `POST /refresh`. The action must exist, the repository must be in the configured list, and a current alert with that repo and URL must have a kind listed in `kinds`. Otherwise the answer is 404.
- The server then POSTs `{"action": "<id>", "alert": {"repo", "kind", "severity", "subject", "detail", "url", "since"}, "requestedAt": "<RFC 3339>"}` to `webhook.url`. When `token_env` is set, the request carries `Authorization: Bearer <token>`. The server never logs the token and never follows redirects.
- A 2xx answer succeeds. If its body is JSON `{"state", "label", "link"}`, the UI shows it as the status of that alert. A non-2xx answer becomes an error message with the upstream body cut to 300 characters. An unreachable webhook gives 502.
- `GET /api/state` gains `actions` (`id`, `label`, `confirm`, `kinds`; no URLs and no tokens). When `status_url` is set, the server also fetches it with `GET` (same bearer token) and expects a JSON list of `{"repo", "url", "state", "label", "link"}` rows, newest first. It matches rows to alerts by repo and URL and adds `actionStatus` to each alert, keyed by action id. The result is cached for 10 seconds. If `status_url` is down, the state omits `actionStatus` and sets `actionsError`.
- A `link` that is not an http or https address is dropped.

## Metrics

All metrics use the `ghtracker_` prefix. Gauges are computed at scrape time.

| Metric                                                                                                         | Labels                     |
| -------------------------------------------------------------------------------------------------------------- | -------------------------- |
| `repo_up`, `repo_health` (0 ok, 1 warn, 2 crit)                                                                | repo                       |
| `repo_open_issues`, `repo_open_prs`, `repo_open_prs_failing_checks`, `repo_stale_issues`                       | repo                       |
| `repo_oldest_pr_age_seconds`, `repo_last_refresh_timestamp_seconds`                                            | repo                       |
| `pr_age_seconds`, `pr_checks_failing`                                                                          | repo, number               |
| `workflow_failing`, `workflow_running`, `workflow_last_run_timestamp_seconds`                                  | repo, workflow, background |
| `release_info`, `release_published_timestamp_seconds`                                                          | repo, version, url         |
| `alerts`                                                                                                       | severity                   |
| `refreshes_total`, `repo_refresh_errors_total`, `last_refresh_duration_seconds`, `github_rate_limit_remaining` | none                       |
| `action_requests_total` (counter; result is `ok`, `rejected` or `error`; only with `actions`)                  | action, result             |

Example alert rules:

```yaml
- alert: GhtrackerWorkflowFailing
  expr: ghtracker_workflow_failing == 1
  for: 10m
- alert: GhtrackerPRWaiting
  expr: ghtracker_pr_age_seconds > 3 * 86400
- alert: GhtrackerRepoDown
  expr: ghtracker_repo_up == 0
  for: 30m
- alert: GhtrackerStale
  expr: time() - ghtracker_last_refresh_timestamp_seconds > 13 * 3600
- alert: GhtrackerRateLimitLow
  expr: ghtracker_github_rate_limit_remaining < 500
```

## GitHub rate limits

The program stays well below the limits of an installation token (at least 5,000 requests per hour):

- One refresh costs about 6 requests per repo, plus 2 per open PR and 1 per active workflow. The default interval is 6 hours.
- Conditional requests with `ETag` make unchanged data free: a 304 reply does not count against the primary limit.
- Concurrency defaults to 2 repos at a time. Pagination stops at `max_items`.
- Requests stop while fewer than 100 remain in the window, until the window resets.
- On a primary or secondary limit response, the client honours `Retry-After` or the reset time, and the current refresh stops. It does not retry. Repos keep their last good data.
- Transient 5xx errors retry twice with a short back-off. Other errors do not retry.
- `refresh_interval` has a one-minute minimum.

## Limits of the workflow view

The program lists default-branch runs only. Runs of a workflow that never ran on the default branch do not appear. Pull request runs appear as PR checks.

## Docker

Run the published image, or build your own with the commands below.

```sh
docker run --rm -p 8080:8080 \
  -v "$PWD/config.yaml:/config/config.yaml:ro" \
  -e GHTRACKER_PRIVATE_KEY="$(cat app.pem)" \
  ghcr.io/lukaszraczylo/ghtracker:latest
```

To build the image yourself:

```sh
docker build -t ghtracker .
docker run --rm -p 8080:8080 \
  -v "$PWD/config.yaml:/config/config.yaml:ro" \
  -e GHTRACKER_PRIVATE_KEY="$(cat app.pem)" \
  ghtracker
```

The image is one static binary, with the UI embedded, on `distroless/static:nonroot`. The config must set `listen: ":8080"` so the port is reachable from outside the container. Build and push the image yourself, then set it in `deploy/kubernetes/deployment.yaml`.

## Kubernetes

`deploy/kubernetes/` holds a Kustomize base:

| File                                         | Purpose                                                                                            |
| -------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| `externalsecret.yaml`                        | Reads the App private key from your secret store through an External Secrets `ClusterSecretStore`. |
| `configmap.yaml`                             | The config file, with the App ID, installation ID and repo list.                                   |
| `deployment.yaml`                            | One replica, non-root, read-only root filesystem, `GHTRACKER_PRIVATE_KEY` from the secret.         |
| `service.yaml`, `ingressroute.yaml`          | Service on port 80 and a Traefik route. Replace the host.                                          |
| `servicemonitor.yaml`, `prometheusrule.yaml` | Scrape every 5 minutes and five starter alerts.                                                    |

```sh
kustomize build deploy/kubernetes | kubectl diff -f -
```

Commit the manifests to the GitOps repo and let ArgoCD sync them. Do not use `kubectl apply`.

Keep the replica count at 1. Each replica refreshes on its own and spends GitHub API budget. The dashboard has no authentication, so keep the route on an internal entrypoint or add an auth middleware.

## Frontend

The UI is a Vue 3 single-page app in `src/`: Vite, Pinia, Tailwind CSS 4, shadcn-vue components and Font Awesome icons. `pnpm build` writes it to `dist/`, and the Go binary embeds that directory with `go:embed`, so the result is one file.

The server returns `503 UI is not built` at `/` until you run `pnpm build`, because `go build` embeds whatever `dist/` holds. `make build` runs both steps.

The UI reads one JSON document from `/api/state` every 60 seconds. The Go side evaluates alert rules, and the UI only groups and displays them. Ages advance in the browser between polls.

## Releases

Every push to `main` runs the shared release workflow from [`lukaszraczylo/shared-actions`](https://github.com/lukaszraczylo/shared-actions). It runs the tests, calculates the version from the commit messages with `semver-generator`, then runs GoReleaser. The result is a GitHub release with binaries and checksums, and a container image on `ghcr.io`.

`semver.yaml` sets the rules: `fix`, `perf`, `refactor` and other maintenance types bump the patch number, `feat` bumps the minor number, and a `Semver-Major:` trailer line is the only way to start a major release.

`workflow-prepare.sh` builds the UI before the Go build, because the binary embeds `dist/`.

## Development

```sh
make dev     # Go server on :8080 plus Vite with hot reload on :5173
make test    # vitest and go test -race
make lint    # eslint, vue-tsc and golangci-lint
```

The Dockerfile builds the UI in a Node stage, then builds the Go binary with the result.
