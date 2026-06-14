# syntax=docker/dockerfile:1.7

# --- gbxa builder ----------------------------------------------------------
# Compiles cmd/gbxa from the repo root (build context = repo root). Produces
# a static binary that gets installed at /usr/local/bin/gbxa in the final
# image; the seven per-agent symlinks are created in the final stage.
FROM golang:1.25-alpine AS gbxa-build
WORKDIR /src
COPY go.mod go.sum* ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/gbxa ./cmd/gbxa

# --- Final image -----------------------------------------------------------
FROM debian:bookworm-slim

ARG HOST_UID=501
ARG HOST_GID=20

ENV DEBIAN_FRONTEND=noninteractive \
     LANG=en_US.UTF-8 \
     LC_ALL=en_US.UTF-8

# --- Bootstrap curl, add external apt sources -------------------------------
# Both repo keys are fetched straight into /etc/apt/keyrings: the gh keyring
# ships in binary form, and apt (>= 2.4) accepts NodeSource's ASCII-armored
# key as-is via a .asc signed-by path - no gnupg/dearmor needed. Writing the
# nodesource.list by hand replaces the setup_NN.x script, which re-ran
# apt-get update and left ~20MB of lists baked into this layer.
ARG NODE_MAJOR=22
RUN apt-get update \
     && apt-get install -y --no-install-recommends ca-certificates curl \
     && rm -rf /var/lib/apt/lists/* \
     && install -d -m 755 /etc/apt/keyrings \
     && curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key \
     -o /etc/apt/keyrings/nodesource.asc \
     && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/nodesource.asc] https://deb.nodesource.com/node_${NODE_MAJOR}.x nodistro main" \
     > /etc/apt/sources.list.d/nodesource.list \
     && curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
     -o /etc/apt/keyrings/githubcli-archive-keyring.gpg \
     && chmod 644 /etc/apt/keyrings/githubcli-archive-keyring.gpg \
     && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
     > /etc/apt/sources.list.d/github-cli.list

# --- All packages in one pass -----------------------------------------------
# Deliberately absent: build-essential (agents install via prebuilt npm
# packages / Python wheels), sudo+gosu (the container runs as 501:20 with
# CapDrop=ALL and no-new-privileges, so privilege elevation can never work),
# pip+venv (uv owns Python tooling). libcap2-bin stays: tests use capsh.
RUN apt-get update \
     && apt-get install -y --no-install-recommends \
     git \
     tini \
     locales \
     jq fzf ripgrep fd-find \
     unzip xz-utils libcap2-bin \
     nodejs \
     python3 \
     gh \
     && sed -i '/^# *en_US.UTF-8 UTF-8/s/^# *//' /etc/locale.gen \
     && locale-gen \
     && rm -rf /var/lib/apt/lists/*

# npm global prefix → ~/.npm (bind-mounted state dir). Agents install here
# at first use via `npm install -g`; binaries persist across container restarts.
ENV NPM_CONFIG_PREFIX=/home/gbx/.npm

# --- uv ---------------------------------------------------------------------
RUN curl -fsSL https://astral.sh/uv/install.sh \
     | UV_INSTALL_DIR=/usr/local/bin sh \
     && /usr/local/bin/uv --version

# --- gbxa: agent wrapper binary + per-agent symlinks ------------------------
# A single Go binary dispatched by argv[0]. Each of the 7 agent names is a
# symlink to /usr/local/bin/gbxa, as is gbx-stack. `gbxa update <agent>`
# remains as a subcommand for the host-side `gbx update <agent>` to drive.
RUN install -d /opt/gbx /etc/glovebox
COPY --from=gbxa-build /out/gbxa /usr/local/bin/gbxa
RUN ln -s /usr/local/bin/gbxa /usr/local/bin/claude \
  && ln -s /usr/local/bin/gbxa /usr/local/bin/codex \
  && ln -s /usr/local/bin/gbxa /usr/local/bin/opencode \
  && ln -s /usr/local/bin/gbxa /usr/local/bin/pi \
  && ln -s /usr/local/bin/gbxa /usr/local/bin/gemini \
  && ln -s /usr/local/bin/gbxa /usr/local/bin/aider \
  && ln -s /usr/local/bin/gbxa /usr/local/bin/hermes \
  && ln -s /usr/local/bin/gbxa /usr/local/bin/gbx-stack

# --- User setup -------------------------------------------------------------
RUN groupmod -g 9020 dialout \
     && groupadd -g "${HOST_GID}" gbx \
     && useradd -m -u "${HOST_UID}" -g gbx -s /bin/bash gbx \
     && install -d -o gbx -g gbx \
     "${NPM_CONFIG_PREFIX}" \
     /home/gbx/.cache \
     /home/gbx/.claude \
     /home/gbx/.codex \
     /home/gbx/.aider \
     /home/gbx/.pi \
     /home/gbx/.gemini \
     /home/gbx/.hermes \
     /home/gbx/.local \
     /home/gbx/.local/bin \
     /home/gbx/.local/share \
     /home/gbx/.local/share/opencode \
     /home/gbx/.local/share/uv-tools \
     /home/gbx/.shell-history \
     /workspace

# PATH: state-backed dirs first so real agent binaries shadow wrappers after
# first install, eliminating any per-invocation overhead.
ENV PATH="/home/gbx/.local/bin:${NPM_CONFIG_PREFIX}/bin:/usr/local/bin:/usr/bin:/bin"

# Debian login shells re-initialize PATH from /etc/profile, erasing the ENV
# above. This profile.d script restores the state-backed dirs idempotently.
RUN printf '#!/bin/sh\ncase ":$PATH:" in\n  *":/home/gbx/.local/bin:"*) ;;\n  *) export PATH="/home/gbx/.local/bin:/home/gbx/.npm/bin:$PATH" ;;\nesac\n' \
     > /etc/profile.d/glovebox-paths.sh

# --- Helpers + entrypoint ---------------------------------------------------
COPY docker/agent-auth /usr/local/bin/agent-auth
RUN chmod +x /usr/local/bin/agent-auth

# Persistent bash history. .shell-history is bind-mounted from the per-project
# state dir, so the file inside survives container recreation.
RUN printf 'export HISTFILE="$HOME/.shell-history/bash_history"\nexport HISTSIZE=10000\nexport HISTFILESIZE=20000\n' \
     > /home/gbx/.bashrc.local \
     && printf '\n[ -f ~/.bashrc.local ] && . ~/.bashrc.local\n' >> /home/gbx/.bashrc \
     && chown gbx:gbx /home/gbx/.bashrc /home/gbx/.bashrc.local

WORKDIR /workspace
ENTRYPOINT ["/usr/bin/tini","--"]
CMD ["sleep","infinity"]
