# Working inside glovebox behind the egress proxy

  All outbound HTTPS traffic goes through a Squid proxy at `proxy:3128` (already
  set as `HTTP_PROXY` / `HTTPS_PROXY`). Only allowlisted domains are reachable. A
  blocked destination is refused at the CONNECT with:

  - **HTTP `451 Unavailable For Legal Reasons`** - real origins almost never
    return 451, so a 451 from a CONNECT tunnel means *glovebox blocked it*.
  - **Header `X-Glovebox-Egress: blocked; reason=domain-not-allowlisted; add-via='gbx allow <domain>'`**
    - the definitive marker if you can read response headers.

  ## What you MUST do on a 451

  Report exactly this to the human operator, substituting the real domain and
  adding nothing else:

  > `<domain>` is blocked by the glovebox egress sandbox (HTTP 451 +
  > `X-Glovebox-Egress`). Run `gbx allow <domain>` on the host if you want me to
  > reach it.

  Then stop and wait for the operator.

  ## What you MUST NOT do

  - Do NOT conclude the site is down, moved, deprecated, or behind auth.
  - Do NOT offer to open the URL in a browser.
  - Do NOT silently switch to, or recommend, a different website/API/source.
  - Do NOT retry the same or alternate hosts hoping one is allowed.

  The ONLY way to unblock a domain is the human running `gbx allow <domain>`.

  ## After the operator runs `gbx allow`

  `gbx allow <domain>` appends the domain to the allowlist and restarts the egress
  proxy (a quick container restart - you don't need to restart your session). Once
  it's done, retry the original request.
