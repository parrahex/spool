<p align="center">
  <img src="mylogo.png" alt="Spool" width="200"/>
</p>

<h1 align="center">Spool</h1>

<p align="center">Run Docker jobs from a CLI or a Slack channel.</p>

<p align="center">
  <a href="https://go.dev"><img src="https://img.shields.io/badge/go-1.26-00ADD8?logo=go" alt="Go 1.27"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-green" alt="MIT License"></a>
  <img src="https://img.shields.io/badge/docker-required-2496ED?logo=docker&logoColor=white" alt="Docker required">
  <img src="https://img.shields.io/badge/redis-queue-DC382D?logo=redis&logoColor=white" alt="Redis queue">
</p>

<hr>

## Install

Requires [Go 1.27+](https://go.dev/dl/), [Docker](https://docs.docker.com/get-docker/) with Compose, and [Task v3](https://taskfile.dev/docs/installation). Redis is included via Compose.

```bash
git clone https://github.com/parrahex/spool.git
cd spool
task build
```

## Quick Start

Start Redis and the worker in one terminal:

```bash
task dev
```

No Slack tokens are needed for CLI development. Task starts two workers by default; export `SPOOL_CONCURRENCY` to override it. Press `Ctrl+C` to stop the worker and Redis.

### From the CLI

```bash
# In another terminal:
task run -- --image alpine echo hello world
```

The run command prints each status change, waits for completion, and displays the final result and output. You can still inspect or cancel a job separately with `task status -- <job-id>` and `task cancel -- <job-id>`; `Ctrl+C` stops waiting without cancelling the job.

<p align="center">
  <img src="test-artifacts/cli-demo.gif" alt="CLI demo" width="650"/>
</p>

### From Slack

Start the Slack-enabled development session first:

```bash
export SLACK_APP_TOKEN=xapp-...
export SLACK_BOT_TOKEN=xoxb-...
task dev:slack
```

Mention the bot in any channel or DM it. Attach files to run them:

```
@Spool alpine echo hello world
```

```
@Spool python:3.11 python script.py
[script.py attached]
```

```
@Spool golang:1.26 go run main.go
[project.zip attached]
```

The bot replies in a thread with the job id, then posts the exit code and output when the job finishes.

<p align="center">
  <img src="test-artifacts/slack-demo.gif" alt="Slack demo" width="650"/>
</p>

## Development commands

Run `task` or `task --list` to see the available commands.

| Command | Purpose |
| --- | --- |
| `task dev` | Start Redis and the CLI worker |
| `task dev:slack` | Start Redis, the worker, and the Slack bot |
| `task run -- <args>` | Submit a job and wait for its result |
| `task status -- <id>` | Inspect a job separately |
| `task cancel -- <id>` | Cancel a pending or running job |
| `task check` | Build, vet, and test all Go packages |
| `task fmt` | Format all Go packages |
| `task docker:build` | Build the production worker image |

## Features

- **Parallel workers** — a pool of goroutines drains the queue; scale with one env var
- **Crash recovery** — jobs are leased; a dead worker's jobs requeue automatically on the next startup
- **Timeouts & retries** — per-job execution limits, configurable retry attempts
- **Zombie cleanup** — every container is labeled and killed if its worker disappears
- **File attachments** — zips, folders, single files; all extracted and mounted read-only at `/app`
- **Graceful shutdown** — in-flight containers get a context cancel, not a kill

## Configuration

| Variable             | Default           | Description                    |
| -------------------- | ----------------- | ------------------------------ |
| `REDIS_ADDR`         | `localhost:6379`  | Redis address                  |
| `SPOOL_CONCURRENCY`  | `1`               | Number of parallel workers     |
| `SPOOL_SHUTDOWN_TIMEOUT` | `30s`          | Grace period for active jobs   |
| `SLACK_APP_TOKEN`    | —                 | App-level token for Socket Mode |
| `SLACK_BOT_TOKEN`    | —                 | Bot user OAuth token           |

## Slack app setup

Create the app from [slack-app-manifest.yml](slack-app-manifest.yml) at [api.slack.com/apps](https://api.slack.com/apps) → *Create New App* → *From a manifest*. Then install it to your workspace and copy the two tokens.

## License

[MIT](LICENSE)
