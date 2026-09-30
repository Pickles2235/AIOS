package discover

import (
	"sort"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

// Diff compares manifests without consulting the repository.  Matching a
// removed and added path by an unambiguous hash is deliberately the only
// rename heuristic: it is deterministic and never guesses a rename.
func Diff(previous, current []model.File) []model.FileChange {
	old := map[string]model.File{}
	new := map[string]model.File{}
	for _, f := range previous {
		old[f.Path] = f
	}
	for _, f := range current {
		new[f.Path] = f
	}
	var added, removed []model.File
	var out []model.FileChange
	for path, f := range new {
		if p, ok := old[path]; !ok {
			added = append(added, f)
		} else if p.SHA256 != f.SHA256 {
			out = append(out, model.FileChange{Kind: "modified", Path: path, SHA256: f.SHA256})
		}
	}
	for path, f := range old {
		if _, ok := new[path]; !ok {
			removed = append(removed, f)
		}
	}
	byHash := map[string][]model.File{}
	for _, f := range added {
		byHash[f.SHA256] = append(byHash[f.SHA256], f)
	}
	used := map[string]bool{}
	for _, f := range removed {
		if candidates := byHash[f.SHA256]; len(candidates) == 1 {
			n := candidates[0]
			used[n.Path] = true
			out = append(out, model.FileChange{Kind: "renamed", Path: n.Path, OldPath: f.Path, SHA256: n.SHA256})
		} else {
			out = append(out, model.FileChange{Kind: "removed", Path: f.Path, SHA256: f.SHA256})
		}
	}
	for _, f := range added {
		if !used[f.Path] {
			out = append(out, model.FileChange{Kind: "added", Path: f.Path, SHA256: f.SHA256})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path == out[j].Path {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Path < out[j].Path
	})
	return out
}
