CREATE TABLE schema_metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL);
INSERT INTO schema_metadata(key,value) VALUES ('format','knowledge-ir-v10');
-- These immutable records are the authority.  FTS and graph tables below are
-- explicitly projections and must carry a matching projection_build record.
CREATE TABLE ir_repositories (repository_id TEXT PRIMARY KEY, created_at TEXT NOT NULL);
CREATE TABLE ir_source_revisions (revision_id TEXT PRIMARY KEY, repository_id TEXT NOT NULL REFERENCES ir_repositories(repository_id), source_kind TEXT NOT NULL, source_adapter_version TEXT NOT NULL, git_commit TEXT NOT NULL, content_hash TEXT NOT NULL, extractor_versions TEXT NOT NULL, indexed_at TEXT NOT NULL);
CREATE TABLE ir_locations (location_id TEXT PRIMARY KEY, revision_id TEXT NOT NULL REFERENCES ir_source_revisions(revision_id), source_id TEXT NOT NULL, path TEXT NOT NULL, file_sha256 TEXT NOT NULL, start_byte INTEGER NOT NULL, end_byte INTEGER NOT NULL, start_line INTEGER NOT NULL, start_column INTEGER NOT NULL, end_line INTEGER NOT NULL, end_column INTEGER NOT NULL, CHECK(start_byte >= 0 AND end_byte >= start_byte));
CREATE TABLE ir_entities (canonical_entity_id TEXT PRIMARY KEY, repository_id TEXT NOT NULL REFERENCES ir_repositories(repository_id), kind TEXT NOT NULL, normalized_identity TEXT NOT NULL, created_at TEXT NOT NULL, UNIQUE(repository_id,kind,normalized_identity));
CREATE TABLE ir_entity_aliases (canonical_entity_id TEXT NOT NULL REFERENCES ir_entities(canonical_entity_id), alias TEXT NOT NULL, normalization TEXT NOT NULL, location_id TEXT NOT NULL REFERENCES ir_locations(location_id), PRIMARY KEY(canonical_entity_id,alias,normalization,location_id));
CREATE TABLE ir_facts (fact_id TEXT PRIMARY KEY, subject_canonical_entity_id TEXT NOT NULL REFERENCES ir_entities(canonical_entity_id), predicate TEXT NOT NULL, object_canonical_entity_id TEXT NOT NULL REFERENCES ir_entities(canonical_entity_id), derivation TEXT NOT NULL, UNIQUE(subject_canonical_entity_id,predicate,object_canonical_entity_id,derivation));
CREATE TABLE ir_fact_observations (fact_observation_id TEXT PRIMARY KEY, fact_id TEXT NOT NULL REFERENCES ir_facts(fact_id), revision_id TEXT NOT NULL REFERENCES ir_source_revisions(revision_id), evidence_id TEXT NOT NULL, location_id TEXT NOT NULL REFERENCES ir_locations(location_id), extractor_name TEXT NOT NULL, extractor_version TEXT NOT NULL, confidence REAL NOT NULL CHECK(confidence >= 0 AND confidence <= 1), indexed_at TEXT NOT NULL, UNIQUE(fact_id,revision_id,evidence_id));
CREATE TABLE projection_builds (
 projection_build_id TEXT PRIMARY KEY,
 projection_kind TEXT NOT NULL CHECK(projection_kind IN ('lookup','lexical','graph','path','ui','cache','vector','landmarks')),
 projection_schema_version TEXT NOT NULL,
 catalog_revision_id TEXT NOT NULL REFERENCES catalog_revisions(catalog_revision_id) ON DELETE CASCADE,
 source_ir_fingerprint TEXT NOT NULL,
 builder_name TEXT NOT NULL, builder_version TEXT NOT NULL,
 input_fingerprint TEXT NOT NULL, projection_fingerprint TEXT NOT NULL,
 created_at TEXT NOT NULL, activated_at TEXT,
 state TEXT NOT NULL CHECK(state IN ('staged','ready','unavailable','failed','retired','disabled')),
 record_counts TEXT NOT NULL, validation_summary TEXT NOT NULL, rebuild_reason TEXT NOT NULL
);
CREATE TABLE active_projection_builds (
 projection_kind TEXT PRIMARY KEY,
 projection_build_id TEXT NOT NULL UNIQUE REFERENCES projection_builds(projection_build_id) ON DELETE RESTRICT
);
CREATE TABLE generations (generation_id TEXT PRIMARY KEY, repo_id TEXT NOT NULL, root TEXT NOT NULL, git_commit TEXT NOT NULL, branch TEXT NOT NULL, dirty INTEGER NOT NULL CHECK(dirty IN (0,1)), untracked_count INTEGER NOT NULL, content_hash TEXT NOT NULL, file_count INTEGER NOT NULL, total_bytes INTEGER NOT NULL, indexed_at TEXT NOT NULL, extractor_versions TEXT NOT NULL, created_at TEXT NOT NULL, activated_at TEXT NOT NULL, source_kind TEXT NOT NULL, source_adapter_version TEXT NOT NULL);
CREATE TRIGGER generations_immutable_update BEFORE UPDATE ON generations BEGIN SELECT RAISE(ABORT, 'published generations are immutable'); END;
CREATE TABLE active_generations (repo_id TEXT PRIMARY KEY, generation_id TEXT NOT NULL UNIQUE REFERENCES generations(generation_id));
CREATE TABLE catalog_revisions (catalog_revision_id TEXT PRIMARY KEY, created_at TEXT NOT NULL, activated_at TEXT NOT NULL);
CREATE TABLE catalog_revision_members (catalog_revision_id TEXT NOT NULL REFERENCES catalog_revisions(catalog_revision_id) ON DELETE CASCADE, repo_id TEXT NOT NULL, generation_id TEXT NOT NULL REFERENCES generations(generation_id) ON DELETE RESTRICT, PRIMARY KEY(catalog_revision_id,repo_id), UNIQUE(catalog_revision_id,generation_id));
CREATE TABLE active_catalog_revision (singleton INTEGER PRIMARY KEY CHECK(singleton=1), catalog_revision_id TEXT NOT NULL UNIQUE REFERENCES catalog_revisions(catalog_revision_id));
CREATE TABLE approved_ownership (repository_id TEXT NOT NULL, coordinate TEXT NOT NULL, owner TEXT NOT NULL, PRIMARY KEY(repository_id,coordinate));
-- Durable orchestration state. Events are append-only audit records; queues
-- are the resumable per-repository command state.
CREATE TABLE ingestion_events (
 event_id TEXT PRIMARY KEY, repository_id TEXT NOT NULL, event_type TEXT NOT NULL,
 source_revision TEXT NOT NULL DEFAULT '', target_revision TEXT NOT NULL DEFAULT '',
 manifest_fingerprint TEXT NOT NULL, timestamp TEXT NOT NULL, attempt INTEGER NOT NULL,
 status TEXT NOT NULL, failure_diagnostic TEXT NOT NULL DEFAULT '',
 UNIQUE(repository_id,event_type,target_revision,manifest_fingerprint,attempt)
);
CREATE TABLE ingestion_queues (
 repository_id TEXT PRIMARY KEY, current_revision TEXT NOT NULL DEFAULT '',
 pending_revision TEXT NOT NULL DEFAULT '', manifest_fingerprint TEXT NOT NULL DEFAULT '',
 state TEXT NOT NULL, attempt INTEGER NOT NULL DEFAULT 0, failure_diagnostic TEXT NOT NULL DEFAULT '',
 selected_at TEXT NOT NULL DEFAULT '', activated_at TEXT NOT NULL DEFAULT '', completed_at TEXT NOT NULL DEFAULT ''
);
-- The ledger is append-only.  Resolution updates only lifecycle fields; the
-- original safe event payload remains an audit record.
CREATE TABLE diagnostic_events (
 diagnostic_id TEXT PRIMARY KEY, code TEXT NOT NULL, severity TEXT NOT NULL CHECK(severity IN ('info','warning','error')),
 timestamp TEXT NOT NULL, repository_id TEXT NOT NULL DEFAULT '', revision_id TEXT NOT NULL DEFAULT '', generation_id TEXT NOT NULL DEFAULT '',
 path TEXT NOT NULL DEFAULT '', projection_kind TEXT NOT NULL DEFAULT '', query_fingerprint TEXT NOT NULL DEFAULT '', handle TEXT NOT NULL DEFAULT '',
 remediation TEXT NOT NULL DEFAULT '', metadata_json TEXT NOT NULL DEFAULT '{}', resolved_at TEXT NOT NULL DEFAULT '', resolution TEXT NOT NULL DEFAULT ''
);
CREATE INDEX diagnostic_events_active ON diagnostic_events(resolved_at,code,timestamp);
CREATE TABLE ir_delta_records (
 delta_id TEXT PRIMARY KEY, repository_id TEXT NOT NULL, source_generation_id TEXT NOT NULL DEFAULT '', target_generation_id TEXT NOT NULL DEFAULT '',
 source_revision TEXT NOT NULL DEFAULT '', target_revision TEXT NOT NULL, manifest_fingerprint TEXT NOT NULL,
 created_at TEXT NOT NULL, activated_at TEXT NOT NULL DEFAULT '', invalidation_scope TEXT NOT NULL, counts_json TEXT NOT NULL
);
CREATE TABLE ir_delta_changes (
 delta_id TEXT NOT NULL REFERENCES ir_delta_records(delta_id) ON DELETE CASCADE, change_kind TEXT NOT NULL,
 canonical_id TEXT NOT NULL, derivation TEXT NOT NULL, invalidation_reason TEXT NOT NULL,
 PRIMARY KEY(delta_id,change_kind,canonical_id)
);
CREATE TABLE ir_fact_lifecycle (
 fact_id TEXT NOT NULL REFERENCES ir_facts(fact_id), generation_id TEXT NOT NULL REFERENCES generations(generation_id),
 state TEXT NOT NULL CHECK(state IN ('active','inactive')), reason TEXT NOT NULL, delta_id TEXT REFERENCES ir_delta_records(delta_id),
 PRIMARY KEY(fact_id,generation_id)
);
CREATE TABLE generation_staging (generation_id TEXT PRIMARY KEY, repo_id TEXT NOT NULL, root TEXT NOT NULL, git_commit TEXT NOT NULL, branch TEXT NOT NULL, dirty INTEGER NOT NULL CHECK(dirty IN (0,1)), untracked_count INTEGER NOT NULL, content_hash TEXT NOT NULL, file_count INTEGER NOT NULL, total_bytes INTEGER NOT NULL, indexed_at TEXT NOT NULL, extractor_versions TEXT NOT NULL, created_at TEXT NOT NULL, source_kind TEXT NOT NULL, source_adapter_version TEXT NOT NULL);
CREATE TABLE staged_compiler_diagnostics (generation_id TEXT NOT NULL REFERENCES generation_staging(generation_id) ON DELETE CASCADE, language TEXT NOT NULL, code TEXT NOT NULL, message TEXT NOT NULL, path TEXT NOT NULL DEFAULT '');
CREATE TABLE staged_coverage_runs (coverage_id TEXT PRIMARY KEY, generation_id TEXT NOT NULL REFERENCES generation_staging(generation_id) ON DELETE CASCADE, repository_id TEXT NOT NULL, source_revision TEXT NOT NULL, source_fingerprint TEXT NOT NULL, extractor_versions TEXT NOT NULL, status TEXT NOT NULL CHECK(status IN ('complete','incomplete')), completed_at TEXT NOT NULL, diagnostic TEXT NOT NULL DEFAULT '');
CREATE TABLE staged_coverage_entries (coverage_id TEXT NOT NULL REFERENCES staged_coverage_runs(coverage_id) ON DELETE CASCADE, path TEXT NOT NULL, language TEXT NOT NULL DEFAULT '', classification TEXT NOT NULL DEFAULT '', outcome TEXT NOT NULL CHECK(outcome IN ('included','excluded','unreadable','unsupported')), reason TEXT NOT NULL DEFAULT '', capability TEXT NOT NULL DEFAULT '', diagnostic TEXT NOT NULL DEFAULT '', PRIMARY KEY(coverage_id,path,outcome,reason));
CREATE TABLE staged_source_files (generation_id TEXT NOT NULL REFERENCES generation_staging(generation_id) ON DELETE CASCADE, source_id TEXT NOT NULL, repo_id TEXT NOT NULL, path TEXT NOT NULL, sha256 TEXT NOT NULL, size INTEGER NOT NULL, language TEXT NOT NULL, classification TEXT NOT NULL, content TEXT NOT NULL, PRIMARY KEY(generation_id, source_id), UNIQUE(generation_id, path));
CREATE TABLE staged_symbols (generation_id TEXT NOT NULL REFERENCES generation_staging(generation_id) ON DELETE CASCADE, repo_id TEXT NOT NULL, path TEXT NOT NULL, name TEXT NOT NULL, kind TEXT NOT NULL, identity TEXT NOT NULL DEFAULT '', start_byte INTEGER NOT NULL, end_byte INTEGER NOT NULL, start_line INTEGER NOT NULL, start_column INTEGER NOT NULL, end_line INTEGER NOT NULL, end_column INTEGER NOT NULL, extractor TEXT NOT NULL, confidence REAL NOT NULL CHECK(confidence >= 0 AND confidence <= 1));
CREATE TABLE staged_edges (generation_id TEXT NOT NULL REFERENCES generation_staging(generation_id) ON DELETE CASCADE, repo_id TEXT NOT NULL, path TEXT NOT NULL, source TEXT NOT NULL, target TEXT NOT NULL, source_identity TEXT NOT NULL DEFAULT '', target_identity TEXT NOT NULL DEFAULT '', kind TEXT NOT NULL, start_byte INTEGER NOT NULL, end_byte INTEGER NOT NULL, start_line INTEGER NOT NULL, start_column INTEGER NOT NULL, end_line INTEGER NOT NULL, end_column INTEGER NOT NULL, resolver TEXT NOT NULL, derivation TEXT NOT NULL DEFAULT 'syntax_derived', confidence REAL NOT NULL CHECK(confidence >= 0 AND confidence <= 1));
CREATE TABLE staged_invalidations (generation_id TEXT NOT NULL REFERENCES generation_staging(generation_id) ON DELETE CASCADE, kind TEXT NOT NULL, path TEXT NOT NULL, old_path TEXT NOT NULL DEFAULT '', sha256 TEXT NOT NULL DEFAULT '', PRIMARY KEY(generation_id,kind,path,old_path));
CREATE TABLE source_files (source_id TEXT PRIMARY KEY, generation_id TEXT NOT NULL REFERENCES generations(generation_id) ON DELETE CASCADE, repo_id TEXT NOT NULL, path TEXT NOT NULL, sha256 TEXT NOT NULL, size INTEGER NOT NULL, language TEXT NOT NULL, classification TEXT NOT NULL, content TEXT NOT NULL, UNIQUE(generation_id, path));
CREATE TABLE compiler_diagnostics (generation_id TEXT NOT NULL REFERENCES generations(generation_id) ON DELETE CASCADE, language TEXT NOT NULL, code TEXT NOT NULL, message TEXT NOT NULL, path TEXT NOT NULL DEFAULT '');
CREATE TABLE coverage_runs (coverage_id TEXT PRIMARY KEY, generation_id TEXT NOT NULL UNIQUE REFERENCES generations(generation_id) ON DELETE CASCADE, repository_id TEXT NOT NULL, source_revision TEXT NOT NULL, source_fingerprint TEXT NOT NULL, extractor_versions TEXT NOT NULL, status TEXT NOT NULL CHECK(status IN ('complete','incomplete')), completed_at TEXT NOT NULL, diagnostic TEXT NOT NULL DEFAULT '');
CREATE TABLE coverage_entries (coverage_id TEXT NOT NULL REFERENCES coverage_runs(coverage_id) ON DELETE CASCADE, path TEXT NOT NULL, language TEXT NOT NULL DEFAULT '', classification TEXT NOT NULL DEFAULT '', outcome TEXT NOT NULL CHECK(outcome IN ('included','excluded','unreadable','unsupported')), reason TEXT NOT NULL DEFAULT '', capability TEXT NOT NULL DEFAULT '', diagnostic TEXT NOT NULL DEFAULT '', PRIMARY KEY(coverage_id,path,outcome,reason));
CREATE TABLE evidence (evidence_id TEXT PRIMARY KEY, generation_id TEXT NOT NULL REFERENCES generations(generation_id) ON DELETE CASCADE, source_id TEXT NOT NULL REFERENCES source_files(source_id) ON DELETE CASCADE, repo_id TEXT NOT NULL, path TEXT NOT NULL, file_sha256 TEXT NOT NULL, start_byte INTEGER NOT NULL, end_byte INTEGER NOT NULL, start_line INTEGER NOT NULL, start_column INTEGER NOT NULL, end_line INTEGER NOT NULL, end_column INTEGER NOT NULL, excerpt TEXT NOT NULL, CHECK(start_byte >= 0 AND end_byte >= start_byte));
CREATE TABLE entities (entity_id TEXT PRIMARY KEY, generation_id TEXT NOT NULL REFERENCES generations(generation_id) ON DELETE CASCADE, repo_id TEXT NOT NULL, kind TEXT NOT NULL, label TEXT NOT NULL, identity TEXT NOT NULL DEFAULT '', path TEXT NOT NULL DEFAULT '', language TEXT NOT NULL DEFAULT '', extractor TEXT NOT NULL DEFAULT 'source', evidence_id TEXT NOT NULL REFERENCES evidence(evidence_id) ON DELETE RESTRICT, canonical_entity_id TEXT NOT NULL REFERENCES ir_entities(canonical_entity_id), UNIQUE(generation_id, identity, kind, label, path));
CREATE TABLE claims (claim_id TEXT PRIMARY KEY, generation_id TEXT NOT NULL REFERENCES generations(generation_id) ON DELETE CASCADE, subject_id TEXT NOT NULL REFERENCES entities(entity_id) ON DELETE CASCADE, predicate TEXT NOT NULL, object_id TEXT NOT NULL REFERENCES entities(entity_id) ON DELETE CASCADE, evidence_id TEXT NOT NULL REFERENCES evidence(evidence_id) ON DELETE RESTRICT, extractor TEXT NOT NULL, derivation TEXT NOT NULL, confidence REAL NOT NULL CHECK(confidence >= 0 AND confidence <= 1), UNIQUE(generation_id, subject_id, predicate, object_id, evidence_id));
CREATE TABLE invalidations (generation_id TEXT NOT NULL REFERENCES generations(generation_id) ON DELETE CASCADE, kind TEXT NOT NULL, path TEXT NOT NULL, old_path TEXT NOT NULL DEFAULT '', sha256 TEXT NOT NULL DEFAULT '', PRIMARY KEY(generation_id,kind,path,old_path));
CREATE TABLE negative_evidence (negative_id TEXT PRIMARY KEY, assertion_type TEXT NOT NULL, normalized_target TEXT NOT NULL, result_state TEXT NOT NULL CHECK(result_state IN ('not_found','unknown')), search_plan_fingerprint TEXT NOT NULL, searched_scope_json TEXT NOT NULL, created_at TEXT NOT NULL, invalidated_at TEXT NOT NULL DEFAULT '', invalidation_reason TEXT NOT NULL DEFAULT '');
CREATE TABLE negative_evidence_dependencies (negative_id TEXT NOT NULL REFERENCES negative_evidence(negative_id) ON DELETE CASCADE, dependency_kind TEXT NOT NULL, dependency_id TEXT NOT NULL, fingerprint TEXT NOT NULL, PRIMARY KEY(negative_id,dependency_kind,dependency_id));
CREATE TABLE canonical_identities (identity_id TEXT PRIMARY KEY, catalog_revision_id TEXT NOT NULL REFERENCES catalog_revisions(catalog_revision_id) ON DELETE CASCADE, kind TEXT NOT NULL, normalized_value TEXT NOT NULL, UNIQUE(catalog_revision_id,kind,normalized_value));
CREATE TABLE identity_memberships (identity_id TEXT NOT NULL REFERENCES canonical_identities(identity_id) ON DELETE CASCADE, entity_id TEXT NOT NULL REFERENCES entities(entity_id) ON DELETE CASCADE, evidence_id TEXT NOT NULL REFERENCES evidence(evidence_id) ON DELETE RESTRICT, original_value TEXT NOT NULL, normalization TEXT NOT NULL, PRIMARY KEY(identity_id,entity_id,evidence_id));
CREATE TABLE cross_claims (cross_claim_id TEXT PRIMARY KEY, catalog_revision_id TEXT NOT NULL REFERENCES catalog_revisions(catalog_revision_id) ON DELETE CASCADE, subject_id TEXT NOT NULL REFERENCES entities(entity_id) ON DELETE CASCADE, predicate TEXT NOT NULL, object_id TEXT NOT NULL REFERENCES entities(entity_id) ON DELETE CASCADE, subject_evidence_id TEXT NOT NULL REFERENCES evidence(evidence_id) ON DELETE RESTRICT, object_evidence_id TEXT NOT NULL REFERENCES evidence(evidence_id) ON DELETE RESTRICT, resolver TEXT NOT NULL, matching_inputs TEXT NOT NULL, derivation TEXT NOT NULL, confidence REAL NOT NULL CHECK(confidence >= 0 AND confidence <= 1), UNIQUE(catalog_revision_id,subject_id,predicate,object_id,subject_evidence_id,object_evidence_id));
CREATE VIRTUAL TABLE source_fts USING fts5(source_id UNINDEXED, generation_id UNINDEXED, repo_id UNINDEXED, path UNINDEXED, content, tokenize='unicode61');
CREATE VIRTUAL TABLE search_fts USING fts5(
 source_id UNINDEXED, entity_id UNINDEXED, evidence_id UNINDEXED,
 generation_id UNINDEXED, repo_id UNINDEXED, language UNINDEXED, classification UNINDEXED,
 source, documentation, path, symbol, strings, logs_errors, configuration,
 tokenize='unicode61'
);
-- Projection records contain only immutable IR handles. They are deliberately
-- duplicated lookup aids, never factual authority.
CREATE TABLE projection_lookup_records (projection_build_id TEXT NOT NULL REFERENCES projection_builds(projection_build_id) ON DELETE CASCADE, generation_id TEXT NOT NULL REFERENCES generations(generation_id) ON DELETE RESTRICT, entity_id TEXT NOT NULL REFERENCES entities(entity_id) ON DELETE RESTRICT, evidence_id TEXT NOT NULL REFERENCES evidence(evidence_id) ON DELETE RESTRICT, PRIMARY KEY(projection_build_id,entity_id));
CREATE TABLE projection_graph_records (projection_build_id TEXT NOT NULL REFERENCES projection_builds(projection_build_id) ON DELETE CASCADE, generation_id TEXT NOT NULL REFERENCES generations(generation_id) ON DELETE RESTRICT, claim_id TEXT NOT NULL REFERENCES claims(claim_id) ON DELETE RESTRICT, subject_id TEXT NOT NULL REFERENCES entities(entity_id) ON DELETE RESTRICT, object_id TEXT NOT NULL REFERENCES entities(entity_id) ON DELETE RESTRICT, evidence_id TEXT NOT NULL REFERENCES evidence(evidence_id) ON DELETE RESTRICT, PRIMARY KEY(projection_build_id,claim_id));
CREATE TABLE projection_cross_graph_records (projection_build_id TEXT NOT NULL REFERENCES projection_builds(projection_build_id) ON DELETE CASCADE, catalog_revision_id TEXT NOT NULL REFERENCES catalog_revisions(catalog_revision_id) ON DELETE CASCADE, cross_claim_id TEXT NOT NULL REFERENCES cross_claims(cross_claim_id) ON DELETE RESTRICT, PRIMARY KEY(projection_build_id,cross_claim_id));
CREATE TABLE projection_path_records (projection_build_id TEXT NOT NULL REFERENCES projection_builds(projection_build_id) ON DELETE CASCADE, generation_id TEXT NOT NULL REFERENCES generations(generation_id) ON DELETE RESTRICT, claim_id TEXT NOT NULL REFERENCES claims(claim_id) ON DELETE RESTRICT, subject_id TEXT NOT NULL REFERENCES entities(entity_id) ON DELETE RESTRICT, object_id TEXT NOT NULL REFERENCES entities(entity_id) ON DELETE RESTRICT, PRIMARY KEY(projection_build_id,claim_id));
CREATE TABLE projection_ui_nodes (projection_build_id TEXT NOT NULL REFERENCES projection_builds(projection_build_id) ON DELETE CASCADE, generation_id TEXT NOT NULL REFERENCES generations(generation_id) ON DELETE RESTRICT, entity_id TEXT NOT NULL REFERENCES entities(entity_id) ON DELETE RESTRICT, evidence_id TEXT NOT NULL REFERENCES evidence(evidence_id) ON DELETE RESTRICT, PRIMARY KEY(projection_build_id,entity_id));
CREATE TABLE projection_ui_edges (projection_build_id TEXT NOT NULL REFERENCES projection_builds(projection_build_id) ON DELETE CASCADE, generation_id TEXT NOT NULL REFERENCES generations(generation_id) ON DELETE RESTRICT, claim_id TEXT NOT NULL REFERENCES claims(claim_id) ON DELETE RESTRICT, PRIMARY KEY(projection_build_id,claim_id));
-- Vector rows are derived retrieval aids. Their canonical entity/evidence
-- references are re-resolved at read time and embeddings are never evidence.
CREATE TABLE vector_embedding_cache (embedding_id TEXT PRIMARY KEY, model_identity TEXT NOT NULL, dimensions INTEGER NOT NULL, input_fingerprint TEXT NOT NULL, vector_json TEXT NOT NULL, created_at TEXT NOT NULL, UNIQUE(model_identity,dimensions,input_fingerprint));
CREATE TABLE projection_vector_records (projection_build_id TEXT NOT NULL REFERENCES projection_builds(projection_build_id) ON DELETE CASCADE, record_id TEXT NOT NULL, generation_id TEXT NOT NULL REFERENCES generations(generation_id) ON DELETE RESTRICT, entity_id TEXT NOT NULL REFERENCES entities(entity_id) ON DELETE RESTRICT, evidence_id TEXT NOT NULL REFERENCES evidence(evidence_id) ON DELETE RESTRICT, embedding_id TEXT NOT NULL REFERENCES vector_embedding_cache(embedding_id) ON DELETE RESTRICT, input_fingerprint TEXT NOT NULL, PRIMARY KEY(projection_build_id,record_id), UNIQUE(projection_build_id,entity_id,evidence_id));
-- High-value records are a projection: each row carries only canonical IDs and
-- bounded structured values, never source bodies or inferred runtime facts.
CREATE TABLE precomputed_records (
 record_id TEXT NOT NULL, projection_build_id TEXT NOT NULL REFERENCES projection_builds(projection_build_id) ON DELETE CASCADE,
 catalog_revision_id TEXT NOT NULL REFERENCES catalog_revisions(catalog_revision_id) ON DELETE CASCADE,
 record_kind TEXT NOT NULL, repository_id TEXT NOT NULL DEFAULT '', service TEXT NOT NULL DEFAULT '',
 label TEXT NOT NULL, summary_json TEXT NOT NULL, evidence_ids TEXT NOT NULL, claim_ids TEXT NOT NULL,
 dependency_ids TEXT NOT NULL, source_generations TEXT NOT NULL, builder_name TEXT NOT NULL,
 builder_version TEXT NOT NULL, confidence REAL NOT NULL CHECK(confidence >= 0 AND confidence <= 1),
 coverage_json TEXT NOT NULL, recomputation_reason TEXT NOT NULL,
 PRIMARY KEY(projection_build_id,record_id)
);
CREATE INDEX precomputed_records_filter ON precomputed_records(projection_build_id,repository_id,service,record_kind,confidence,record_id);
CREATE INDEX generations_repo_activated ON generations(repo_id, activated_at);
CREATE INDEX source_files_active_path ON source_files(generation_id, path);
CREATE INDEX evidence_generation_source_span ON evidence(generation_id, source_id, start_byte, end_byte);
CREATE INDEX entities_generation_label ON entities(generation_id, label, kind, path);
CREATE INDEX claims_subject_adjacency ON claims(generation_id, subject_id, predicate, object_id);
CREATE INDEX claims_object_adjacency ON claims(generation_id, object_id, predicate, subject_id);
CREATE INDEX catalog_revision_members_generation ON catalog_revision_members(generation_id);
CREATE INDEX identity_memberships_entity ON identity_memberships(entity_id);
CREATE INDEX cross_claims_subject_adjacency ON cross_claims(catalog_revision_id,subject_id,predicate,object_id);
CREATE INDEX cross_claims_object_adjacency ON cross_claims(catalog_revision_id,object_id,predicate,subject_id);
CREATE INDEX ir_locations_revision_source ON ir_locations(revision_id,source_id,start_byte,end_byte);
CREATE INDEX ir_fact_observations_revision ON ir_fact_observations(revision_id,fact_id);
CREATE INDEX coverage_entries_outcome ON coverage_entries(coverage_id,outcome,reason);
CREATE INDEX negative_evidence_current ON negative_evidence(assertion_type,normalized_target,invalidated_at);
CREATE INDEX projection_builds_catalog_kind ON projection_builds(catalog_revision_id,projection_kind,state);
CREATE INDEX projection_vector_records_lookup ON projection_vector_records(projection_build_id,generation_id,entity_id);
