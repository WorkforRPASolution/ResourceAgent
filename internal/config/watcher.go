package config

import (
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"

	"resourceagent/internal/logger"
)

// Watcher monitors configuration file changes and triggers reload callbacks.
type Watcher struct {
	path     string
	watcher  *fsnotify.Watcher
	callback func(*Config)

	mu       sync.Mutex
	running  bool
	stopChan chan struct{}
}

// NewWatcher creates a new configuration file watcher.
func NewWatcher(path string, callback func(*Config)) (*Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &Watcher{
		path:     path,
		watcher:  w,
		callback: callback,
		stopChan: make(chan struct{}),
	}, nil
}

// Start begins watching for configuration file changes.
func (w *Watcher) Start() error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return nil
	}
	w.running = true
	w.mu.Unlock()

	log := logger.WithComponent("config-watcher")

	// Watch the directory containing the config file
	dir := filepath.Dir(w.path)
	if err := w.watcher.Add(dir); err != nil {
		return err
	}

	log.Info().Str("path", w.path).Msg("Started watching configuration file")

	go w.watch()
	return nil
}

// Stop stops watching for changes.
func (w *Watcher) Stop() error {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return nil
	}
	w.running = false
	w.mu.Unlock()

	close(w.stopChan)
	return w.watcher.Close()
}

func (w *Watcher) watch() {
	log := logger.WithComponent("config-watcher")
	filename := filepath.Base(w.path)

	for {
		select {
		case <-w.stopChan:
			log.Info().Msg("Configuration watcher stopped")
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// Only process events for our config file
			if filepath.Base(event.Name) != filename {
				continue
			}

			// Handle write and create events
			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
				log.Info().Str("event", event.Op.String()).Msg("Configuration file changed, reloading")

				cfg, err := Load(w.path)
				if err != nil {
					log.Error().Err(err).Msg("Failed to reload configuration")
					continue
				}

				if w.callback != nil {
					w.callback(cfg)
				}
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Error().Err(err).Msg("Configuration watcher error")
		}
	}
}

// IsRunning returns whether the watcher is currently running.
func (w *Watcher) IsRunning() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}
