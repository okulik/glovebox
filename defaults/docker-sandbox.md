# Working inside glovebox without Docker

You are running inside the glovebox container. You cannot reach the Docker
daemon and you cannot bind host ports. If you need an auxiliary service
(Redis, Postgres, Neo4j, …) to run or test code, do NOT refuse - use the
`gbx-stack` workflow below.

A short summary of glovebox's sandbox notes (including the egress proxy's
451 / `X-Glovebox-Egress` signal) is already inlined into your top-level
instruction file. This file is the long-form companion - open it when you
need the dev-stack workflow, the manifest validation rules, or any of the
details the summary punts to.

## Proposing a new service

Step 1. Write a draft manifest to a scratch path (e.g. `/tmp/stack.yml`).
A minimal example for Neo4j:

    version: 1
    services:
      neo4j:
        image: neo4j:5
        env:
          NEO4J_AUTH: none
        volumes:
          data: /data

Step 2. Run `gbx-stack propose /tmp/stack.yml`. This POSTs the draft to
the controller and prints an operator hint. You CANNOT apply it yourself.

Step 3. STOP and ask the operator (the human user) to run `gbx stack diff`
and `gbx stack apply` on the host. Wait for them - there is no
programmatic path around this approval step.

Step 4. Run `gbx-stack wait`. It blocks until services are healthy.

Step 5. Talk to services by DNS name and standard ports (`neo4j:7687`,
`redis:6379`, `postgres:5432`). Run `gbx-stack info` to discover the live
host/port map (JSON).

## Operating an already-applied stack

- `gbx-stack status` - health summary.
- `gbx-stack start|stop|reset <svc>` - works without re-approval.
- `gbx-stack logs <svc> [--follow]` - service logs.
- Adding or changing services means propose → ask operator → apply again.

## Manifest constraints (so your draft passes validation)

- `image` must be fully tagged (no `:latest`) and come from an allowed
  registry: `docker.io`, `ghcr.io`, `gcr.io`, `public.ecr.aws`,
  `quay.io`, `mcr.microsoft.com`.
- `volumes` are named only (`<name>: <container-path>`); host binds are
  rejected.
- `env` values may reference `${FOO}` only if `FOO` is on the env
  allowlist.
- `cap_add` is restricted to `IPC_LOCK`, `SYS_NICE`, `SYS_RESOURCE`,
  `DAC_READ_SEARCH`. Anything else (e.g. `NET_ADMIN`, `SYS_ADMIN`,
  `SYS_PTRACE`) is rejected.
- `resources.cpus` ≤ 4, `resources.memory` ≤ 8 GiB.

Run `gbx-stack --help` for the full command list.
