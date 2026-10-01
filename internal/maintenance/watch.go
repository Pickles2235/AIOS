package maintenance

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const maximumWatches = 8192

type watchSet struct {
	mu             sync.Mutex
	watcher        *fsnotify.Watcher
	engine         *Engine
	sources        map[string]Source
	paths          map[string]bool
	done, finished chan struct{}
	once           sync.Once
	closed         bool
}

func newWatchSet(engine *Engine, sources map[string]Source) (*watchSet, error) {
	hasDirect := false
	for _, source := range sources {
		if source.Mode == "direct" {
			hasDirect = true
		}
	}
	if !hasDirect {
		return nil, nil
	}
	watcher, err := fsnotify.NewBufferedWatcher(1024)
	if err != nil {
		return nil, err
	}
	w := &watchSet{watcher: watcher, engine: engine, sources: sources, paths: map[string]bool{}, done: make(chan struct{}), finished: make(chan struct{})}
	if err = w.refresh(); err != nil {
		engine.setWatchError("Some directories are unwatched; periodic reconciliation remains active.")
	} else {
		engine.setWatchError("")
	}
	go w.loop()
	return w, nil
}

func blockedWatchDirectory(name string) bool {
	switch name {
	case ".git", ".gradle", "node_modules", "vendor", "build", "dist", "target":
		return true
	}
	return false
}

func (w *watchSet) refresh() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return fmt.Errorf("watches closed")
	}
	wanted := map[string]bool{}
	var failure error
	for _, source := range w.sources {
		if source.Mode != "direct" {
			continue
		}
		real, err := filepath.EvalSymlinks(source.Path)
		if err != nil || real != source.Path {
			failure = fmt.Errorf("workspace watch unavailable")
			continue
		}
		err = filepath.WalkDir(source.Path, func(path string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if !d.IsDir() {
				return nil
			}
			if path != source.Path && blockedWatchDirectory(d.Name()) {
				return filepath.SkipDir
			}
			if len(wanted) >= maximumWatches {
				return fmt.Errorf("watch directory bound exceeded")
			}
			wanted[path] = true
			if !w.paths[path] {
				if e := w.watcher.Add(path); e != nil {
					return e
				}
				w.paths[path] = true
			}
			return nil
		})
		if err != nil {
			failure = err
		}
	}
	for path := range w.paths {
		if !wanted[path] {
			_ = w.watcher.Remove(path)
			delete(w.paths, path)
		}
	}
	return failure
}

func (w *watchSet) affected(path string) []string {
	ids := []string{}
	for id, source := range w.sources {
		if source.Mode != "direct" {
			continue
		}
		rel, e := filepath.Rel(source.Path, path)
		if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		parts := strings.Split(rel, string(filepath.Separator))
		blocked := false
		for _, part := range parts {
			if blockedWatchDirectory(part) {
				blocked = true
				break
			}
		}
		if !blocked {
			ids = append(ids, id)
		}
	}
	return ids
}

func (w *watchSet) loop() {
	defer close(w.finished)
	batch := map[string]bool{}
	flush := time.NewTicker(50 * time.Millisecond)
	defer flush.Stop()
	refresh := time.NewTicker(30 * time.Second)
	defer refresh.Stop()
	for {
		select {
		case <-w.done:
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				w.streamLost()
				return
			}
			for _, id := range w.affected(event.Name) {
				batch[id] = true
			}
			if event.Op&(fsnotify.Create|fsnotify.Rename) != 0 {
				if info, e := os.Lstat(event.Name); e == nil && info.IsDir() {
					if e = w.refresh(); e != nil {
						w.engine.setWatchError("Watch directory limit or access failure; periodic reconciliation remains active.")
					}
				}
			}
		case _, ok := <-w.watcher.Errors:
			if !ok {
				w.streamLost()
				return
			}
			w.engine.setWatchError("Watch events were lost; reconciling current immutable inputs.")
			for id, source := range w.sources {
				if source.Mode == "direct" {
					_ = w.engine.Request(id, "lost_event_reconcile")
				}
			}
		case <-flush.C:
			for id := range batch {
				_ = w.engine.MarkEdit(id)
				delete(batch, id)
			}
		case <-refresh.C:
			if e := w.refresh(); e != nil {
				w.engine.setWatchError("Watch repair incomplete; periodic reconciliation remains active.")
			}
		}
	}
}

func (w *watchSet) streamLost() {
	select {
	case <-w.done:
		return
	default:
	}
	w.engine.setWatchError("OS watch stream closed; periodic reconciliation remains active.")
	for id, source := range w.sources {
		if source.Mode == "direct" {
			_ = w.engine.Request(id, "lost_event_reconcile")
		}
	}
}

func (w *watchSet) Close() {
	w.once.Do(func() { w.mu.Lock(); w.closed = true; w.mu.Unlock(); close(w.done); _ = w.watcher.Close() })
	<-w.finished
}
