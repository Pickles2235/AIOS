package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// Cross-repository resolution is deliberately a small, syntax-backed join.
// It never models broker delivery, registration, authentication, or topology.
type crossCandidate struct {
	entity, evidence, repo, predicate, label      string
	role, identityKind, normalized, normalization string
	confidence                                    float64
}

func normalizeCross(predicate, label string) (kind, normalized, rule string, ok bool) {
	p := strings.ToUpper(predicate)
	trim := strings.TrimSpace(label)
	switch {
	case p == "PUBLISHES_EVENT" || p == "CONSUMES_EVENT" || p == "LISTENS_EVENT" || p == "SUBSCRIBES_EVENT":
		return "event", trim, "event-exact-case-sensitive", trim != ""
	case p == "PRODUCES_TOPIC" || p == "CONSUMES_TOPIC":
		return "topic", strings.ToLower(trim), "topic-trim-lower", trim != ""
	case p == "SENDS_QUEUE" || p == "LISTENS_QUEUE":
		return "queue", strings.ToLower(trim), "queue-trim-lower", trim != ""
	case p == "DECLARES_EFFECTIVE_ENDPOINT" || p == "INVOKES_API":
		parts := strings.Fields(trim)
		if len(parts) != 2 || !strings.HasPrefix(parts[1], "/") {
			return "", "", "", false
		}
		method := strings.ToUpper(parts[0])
		switch method {
		case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		default:
			return "", "", "", false
		}
		return "http_endpoint", method + " " + cleanPath(parts[1]), "http-method-uppercase-path-exact", true
	case p == "DEFINES_CONFIGURATION" || p == "REFERENCES_CONFIGURATION" || p == "BINDS_CONFIGURATION":
		v := strings.ToLower(strings.NewReplacer("_", ".", "-", ".").Replace(trim))
		v = strings.Join(strings.FieldsFunc(v, func(r rune) bool { return r == '.' }), ".")
		return "configuration", v, "configuration-relaxed-dot", v != ""
	case p == "DEPENDENCY_DECLARATION" || p == "DECLARES_MODULE":
		return "dependency", strings.ToLower(trim), "coordinate-trim-lower", trim != ""
	case p == "ALIASES_SCHEMA":
		return "schema", trim, "schema-exact-case-sensitive", trim != ""
	}
	return "", "", "", false
}
func cleanPath(p string) string {
	if p == "/" {
		return p
	}
	return strings.TrimRight(p, "/")
}

func roleFor(predicate string) string {
	switch strings.ToUpper(predicate) {
	case "PUBLISHES_EVENT", "PRODUCES_TOPIC", "SENDS_QUEUE", "DEFINES_CONFIGURATION", "DECLARES_EFFECTIVE_ENDPOINT", "DECLARES_MODULE":
		return "provider"
	case "CONSUMES_EVENT", "LISTENS_EVENT", "SUBSCRIBES_EVENT", "CONSUMES_TOPIC", "LISTENS_QUEUE", "REFERENCES_CONFIGURATION", "BINDS_CONFIGURATION", "INVOKES_API", "DEPENDENCY_DECLARATION":
		return "consumer"
	}
	return ""
}

func linkCatalog(ctx context.Context, tx *sql.Tx, revision string) error {
	rows, err := tx.QueryContext(ctx, `SELECT c.object_id,c.evidence_id,o.repo_id,c.predicate,o.label,c.confidence
		FROM claims c JOIN entities o ON o.entity_id=c.object_id
		JOIN catalog_revision_members m ON m.generation_id=c.generation_id
		WHERE m.catalog_revision_id=? ORDER BY o.repo_id,c.predicate,o.label,c.claim_id`, revision)
	if err != nil {
		return err
	}
	defer rows.Close()
	var cs []crossCandidate
	for rows.Next() {
		var c crossCandidate
		if err := rows.Scan(&c.entity, &c.evidence, &c.repo, &c.predicate, &c.label, &c.confidence); err != nil {
			return err
		}
		c.role = roleFor(c.predicate)
		c.identityKind, c.normalized, c.normalization, _ = normalizeCross(c.predicate, c.label)
		if c.role != "" && c.identityKind != "" {
			cs = append(cs, c)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	identityIDs := map[string]string{}
	for _, c := range cs {
		key := c.identityKind + "\x00" + c.normalized
		id := identityIDs[key]
		if id == "" {
			id = "i-" + digest(revision, c.identityKind, c.normalized)
			identityIDs[key] = id
			if _, err := tx.ExecContext(ctx, `INSERT INTO canonical_identities VALUES(?,?,?,?)`, id, revision, c.identityKind, c.normalized); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO identity_memberships VALUES(?,?,?,?,?)`, id, c.entity, c.evidence, c.label, c.normalization); err != nil {
			return err
		}
	}
	groups := map[string][]crossCandidate{}
	for _, c := range cs {
		groups[c.identityKind+"\x00"+c.normalized] = append(groups[c.identityKind+"\x00"+c.normalized], c)
	}
	for key, group := range groups {
		_ = key
		for _, a := range group {
			if a.role != "provider" {
				continue
			}
			for _, b := range group {
				if b.role != "consumer" || a.repo == b.repo {
					continue
				}
				predicate := "CROSS_REPOSITORY_LINK"
				switch a.identityKind {
				case "event", "topic", "queue":
					predicate = "EVENT_PRODUCER_CONSUMER"
				case "http_endpoint":
					predicate = "HTTP_CLIENT_ROUTE"
				case "configuration":
					predicate = "CONFIGURATION_DEFINITION_CONSUMER"
				case "dependency":
					predicate = "DEPENDENCY_LOCAL_MODULE"
				}
				confidence := a.confidence
				if b.confidence < confidence {
					confidence = b.confidence
				}
				inputs := fmt.Sprintf("kind=%s;normalized=%s;provider=%s;consumer=%s", a.identityKind, a.normalized, a.label, b.label)
				id := "x-" + digest(revision, a.entity, predicate, b.entity, a.evidence, b.evidence)
				if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO cross_claims VALUES(?,?,?,?,?,?,?,?,?,?,?)`, id, revision, a.entity, predicate, b.entity, a.evidence, b.evidence, "direct-canonical-identity", inputs, "cross_repository_direct_evidence", confidence); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// CrossClaims returns deterministic evidence-backed links for an active graph.
func (s *Store) CrossClaims(ctx context.Context, revision, entity, direction string, predicates []string, limit int) ([]CrossClaim, bool, error) {
	if direction == "" {
		direction = "both"
	}
	if direction != "in" && direction != "out" && direction != "both" {
		return nil, false, fmt.Errorf("direction must be in, out, or both")
	}
	q := `SELECT x.cross_claim_id,x.catalog_revision_id,x.subject_id,x.predicate,x.object_id,x.subject_evidence_id,x.object_evidence_id,x.resolver,x.matching_inputs,x.derivation,x.confidence FROM projection_cross_graph_records p JOIN active_projection_builds a ON a.projection_build_id=p.projection_build_id JOIN cross_claims x ON x.cross_claim_id=p.cross_claim_id WHERE a.projection_kind='graph' AND p.catalog_revision_id=?`
	args := []any{revision}
	if direction == "in" {
		q += " AND x.object_id=?"
		args = append(args, entity)
	} else if direction == "out" {
		q += " AND x.subject_id=?"
		args = append(args, entity)
	} else {
		q += " AND (x.subject_id=? OR x.object_id=?)"
		args = append(args, entity, entity)
	}
	if len(predicates) > 0 {
		q += " AND lower(x.predicate) IN (" + placeholders(len(predicates)) + ")"
		for _, p := range predicates {
			args = append(args, strings.ToLower(p))
		}
	}
	q += " ORDER BY x.predicate,x.subject_id,x.object_id,x.cross_claim_id LIMIT ?"
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var out []CrossClaim
	for rows.Next() {
		var c CrossClaim
		if err := rows.Scan(&c.ID, &c.CatalogRevision, &c.SubjectID, &c.Predicate, &c.ObjectID, &c.SubjectEvidenceID, &c.ObjectEvidenceID, &c.Resolver, &c.MatchingInputs, &c.Derivation, &c.Confidence); err != nil {
			return nil, false, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	truncated := len(out) > limit
	if truncated {
		out = out[:limit]
	}
	return out, truncated, nil
}
func (s *Store) CrossClaimsForLabel(ctx context.Context, revision, label string, limit int) ([]CrossClaim, bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT x.cross_claim_id,x.catalog_revision_id,x.subject_id,x.predicate,x.object_id,x.subject_evidence_id,x.object_evidence_id,x.resolver,x.matching_inputs,x.derivation,x.confidence FROM projection_cross_graph_records p JOIN active_projection_builds a ON a.projection_build_id=p.projection_build_id JOIN cross_claims x ON x.cross_claim_id=p.cross_claim_id JOIN entities s ON s.entity_id=x.subject_id JOIN entities o ON o.entity_id=x.object_id WHERE a.projection_kind='graph' AND p.catalog_revision_id=? AND (lower(s.label)=lower(?) OR lower(o.label)=lower(?)) ORDER BY x.predicate,x.subject_id,x.object_id,x.cross_claim_id LIMIT ?`, revision, label, label, limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var out []CrossClaim
	for rows.Next() {
		var c CrossClaim
		if err := rows.Scan(&c.ID, &c.CatalogRevision, &c.SubjectID, &c.Predicate, &c.ObjectID, &c.SubjectEvidenceID, &c.ObjectEvidenceID, &c.Resolver, &c.MatchingInputs, &c.Derivation, &c.Confidence); err != nil {
			return nil, false, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	tr := len(out) > limit
	if tr {
		out = out[:limit]
	}
	return out, tr, nil
}

type CrossClaim struct {
	ID, CatalogRevision, SubjectID, Predicate, ObjectID, SubjectEvidenceID, ObjectEvidenceID, Resolver, MatchingInputs, Derivation string
	Confidence                                                                                                                     float64
}

func sortCandidates(cs []crossCandidate) {
	sort.Slice(cs, func(i, j int) bool { return cs[i].repo+cs[i].label < cs[j].repo+cs[j].label })
}
func placeholders(n int) string { return strings.TrimRight(strings.Repeat("?,", n), ",") }
