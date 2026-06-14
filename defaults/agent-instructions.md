# Glovebox sandboxes

You are running inside a Glovebox container: you cannot reach the Docker daemon
or bind host ports. If you need an auxiliary service (Redis, Postgres, Neo4j, …)
to build, run, or test code, do NOT refuse and do NOT try to install or run it
yourself. Read `/etc/glovebox/docker-sandbox.md` and follow its `gbx-stack`
workflow - propose a manifest, ask the operator to apply it, then wait. You
cannot apply a stack yourself.

Glovebox restricts outbound network access through a Squid proxy. A request the
sandbox blocks returns **HTTP 451 Unavailable For Legal Reasons** (plus an
`X-Glovebox-Egress` response header). 451 here means *glovebox blocked the
domain* - not that the site is down, deprecated, or auth-gated.

On a 451 you MUST:
1. Read `/etc/glovebox/proxy-sandbox.md` for the full protocol.
2. Tell the human operator, substituting the real domain and adding nothing else:
    "`<domain>` is blocked by the glovebox egress sandbox (HTTP 451). Run
    `gbx allow <domain>` on the host to let me reach it."
3. Then STOP and wait. Do NOT open the URL in a browser, do NOT switch to or
    suggest a different site/source, and do NOT retry - the only unblock is the
    human running `gbx allow <domain>`.
