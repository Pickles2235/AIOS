# Configure a catalog

Catalogs are JSON objects with `version: 1`, 1–100 repository sources,
and optional bounded limits. Unknown fields are rejected. The abbreviated
example below is a valid single-repository catalog. Catalog and registry IDs must match.

```json
{
  "version": 1,
  "sources": [{
    "kind": "repository",
    "id": "frontend",
    "include": ["**/*.ts", "**/*.tsx"],
    "exclude": ["generated/**"]
  }],
  "limits": {
    "max_file_bytes": 1048576,
    "max_files_per_repo": 20000,
    "max_total_bytes_per_repo": 268435456,
    "max_results": 100
  }
}
```

Repository source IDs match `[a-z0-9][a-z0-9._-]{0,62}` and must be unique.
Include/exclude patterns are relative slash-separated globs; a source may
declare at most 64 patterns, each at most 256 bytes.

The catalog is capped at 1 MiB and 100 repositories. Limits must be positive and within the program bounds: file size 16 MiB, files 200,000, total bytes 2 GiB, and results 500. Omitting `limits` selects built-in defaults.

Validate with `./bin/aios catalog validate --config /path/to/catalog.json`.

Select include sets carefully. Filename deny rules reduce accidental indexing of secrets but cannot detect every sensitive value.

## Managed mirror registry

Repository addresses are managed separately from the source-policy catalog.
Record the explicitly approved IDs (1–100), remote addresses, full refs, patterns,
and ownership coordinates in a private inventory. Compile and verify it without
discovering or indexing local checkouts:

```sh
python3 scripts/generate_v1_catalog.py \
  --inventory /private/approved-inventory.json \
  --data-dir /private/agent-data
```

This writes owner-only `catalog.json` and `mirrors.json` files beneath the data
directory after confirming every remote/ref exists. `--skip-remote-check` is
reserved for offline validation and is not a production bootstrap gate.

AIOS clones and fetches only under its data directory; it never indexes local
checkouts directly. Run synchronization from external cron every 15 minutes:

```sh
*/15 * * * * /absolute/path/aios mirrors sync --registry /private/agent-data/mirrors.json --data-dir /private/agent-data
```
