# Catalog format

The top-level fields are `version`, `sources`, and optional `limits` and
`vector`. V1 requires exactly 25 `repository` sources. Their approved Git
addresses and refs live only in the separate agent-managed mirror registry;
catalogs never contain checkout roots or remote addresses.

Each source has `kind`, `id`, optional `include`, and optional `exclude`.
Only `kind: "repository"` is accepted. Each limit is numeric:
`max_file_bytes`, `max_files_per_repo`, `max_total_bytes_per_repo`, and
`max_results`.

Built-in defaults are 1 MiB per file, 20,000 files, 256 MiB per repository, and 100 results. Validation bounds are documented in the how-to guide and enforced before indexing or serving.
