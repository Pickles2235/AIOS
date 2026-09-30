# Safety and provenance

The catalog is the authority for repository identity and roots. Repository content cannot add tools, select another root, or execute commands. Discovery excludes known secret-like paths, symlinks, binary/invalid text, dependencies, and build output; this is a safety backstop, not a data-loss-prevention system.

Git is queried with fixed read-only commands. A snapshot records commit, branch, dirty state, untracked count, per-file hashes, an aggregate content hash, and extractor versions. The indexer compares Git state before and after indexing and refuses to publish on change.

Evidence is scoped to repository ID, relative path, file hash, span, and generation. This makes results auditable and prevents stale evidence from being presented as current after re-indexing.

The SQLite database contains indexed source text and has no at-rest encryption or multi-user authorization. It is intended for one trusted local user. The separate cache database contains no source text or raw requests; it is still owner-only and remains local to the same trusted user.
