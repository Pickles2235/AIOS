# Security model

## Permitted

- Read files below explicitly catalogued, canonical repository roots.
- Execute fixed read-only native Git queries and explicit approved mirror clone/fetch
  into owned data. Source workspaces and remotes are never written.
- Write the SQLite database and its SQLite-managed side files below `--data-dir` during `ingest`.
- Serve a fixed set of read-only tools over local process stdio.
- Serve the browser API on IPv4 loopback with one-use capabilities, HttpOnly
  same-site sessions, matching Origin and CSRF checks.
- Save authenticated bounded setup/branding metadata into fixed owned paths.

## Denied or absent

- Parent-directory repository discovery.
- Symlink traversal or arbitrary path arguments.
- Non-loopback network listeners; outbound model, telemetry or connector calls.
  Explicit Git clone/fetch is the approved remote access boundary.
- Embedded provider token stores and credential collection, VPN, 1Password, Jira,
  or Confluence integrations. The planned daemon uses machine Git/SSH/credential
  helpers configured outside AgentOS; current mirror commands still disable
  helpers/global config, a tracked completion gap.
- Source repository writes, reset/stash/checkout, builds, tests, arbitrary shell
  execution, deployment or agent actions. Mirror mutation is confined to owned data.
- Secret-bearing and customer-data indexing by design; filename/path deny rules are a safety backstop, not a data-loss-prevention product.
- External vector embeddings or model calls. Optional vector work is local-only
  and never sends indexed source outside the machine. The pinned Nomic model
  and llama.cpp helper are embedded in the macOS arm64 executable and are
  materialized owner-only below the agent data directory when used.

## Residual risks

Filename rules cannot identify every sensitive value inside an innocently named source file. Catalog owners must select roots and include patterns that contain source suitable for local indexing. SQLite contains full text for included files and must be protected as source code. The current local product has no at-rest encryption or multi-user authorization. It is intended for a single trusted local user.

AST extractors parse untrusted text in-process through pinned CGO grammars. File, total-byte, result, depth, and query limits bound common resource abuse; they do not replace OS-level process isolation for hostile repositories.

Browser instance/setup mutations are authenticated, same-origin and CSRF-protected.
They accept bounded typed values and write fixed owned metadata/snapshot paths only.
Mirror URL validation rejects credentials and arbitrary Git remote helper schemes.
An existing mirror cannot silently change its approved remote; use a new source ID.
Setup cancellation and restart preserve canonical active generations for retry.
