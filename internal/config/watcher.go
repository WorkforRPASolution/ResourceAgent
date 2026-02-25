package config

import (
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"

	"resourceagent/internal/logger"
)

// FileWatcher monitors a single file for changes and invokes a callback on modification.
type FileWatcher struct {
	path     string
	watcher  *fsnotify.Watcher
	onChange func()

	mu       sync.Mutex
	running  bool
	stopChan chan struct{}
}

// NewFileWatcher creates a generic file watcher that calls onChange when the file is modified.
func NewFileWatcher(path string, onChange func()) (*FileWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &FileWatcher{
		path:     path,
		watcher:  w,
		onChange: onChange,
		stopChan: make(chan struct{}),
	}, nil
}

// Start begins watching for file changes.
func (fw *FileWatcher) Start() error {
	fw.mu.Lock()
	if fw.running {
		fw.mu.Unlock()
		return nil
	}
	fw.running = true
	fw.mu.Unlock()

	log := logger.WithComponent("file-watcher")

	dir := filepath.Dir(fw.path)
	if err := fw.watcher.Add(dir); err != nil {
		return err
	}

	log.Info().Str("path", fw.path).Msg("Started watching file")

	go fw.watch()
	return nil
}

// Stop stops watching for changes.
func (fw *FileWatcher) Stop() error {
	fw.mu.Lock()
	if !fw.running {
		fw.mu.Unlock()
		return nil
	}
	fw.running = false
	fw.mu.Unlock()

	close(fw.stopChan)
	return fw.watcher.Close()
}

func (fw *FileWatcher) watch() {
	log := logger.WithComponent("file-watcher")
	filename := filepath.Base(fw.path)

	for {
		select {
		case <-fw.stopChan:
			log.Info().Str("path", fw.path).Msg("File watcher stopped")
			return

		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}

			if filepath.Base(event.Name) != filename {
				continue
			}

			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
				log.Info().
					Str("path", fw.path).
					Str("event", event.Op.String()).
					Msg("File changed, reloading")

				if fw.onChange != nil {
					fw.onChange()
				}
			}

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			log.Error().Err(err).Str("path", fw.path).Msg("File watcher error")
		}
	}
}

// IsRunning returns whether the watcher is currently running.
func (fw *FileWatcher) IsRunning() bool {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	return fw.running
}

// --- Convenience constructors for typed watchers ---

// Watcher is kept for backward compatibility. It monitors a config file and reloads Config.
type Watcher = FileWatcher

// NewWatcher creates a watcher that loads a full Config on file change.
func NewWatcher(path string, callback func(*Config)) (*FileWatcher, error) {
	return NewFileWatcher(path, func() {
		log := logger.WithComponent("config-watcher")
		cfg, err := Load(path)
		if err != nil {
			log.Error().Err(err).Msg("Failed to reload configuration")
			return
		}
		if callback != nil {
			callback(cfg)
		}
	})
}

// NewMonitorWatcher creates a watcher that loads MonitorConfig on file change.
func NewMonitorWatcher(path string, callback func(*MonitorConfig)) (*FileWatcher, error) {
	return NewFileWatcher(path, func() {
		log := logger.WithComponent("monitor-watcher")
		mc, err := LoadMonitor(path)
		if err != nil {
			log.Error().Err(err).Msg("Failed to reload monitor configuration")
			return
		}
		if callback != nil {
			callback(mc)
		}
	})
}

// NewLoggingWatcher creates a watcher that loads logger.Config on file change.
func NewLoggingWatcher(path string, callback func(*logger.Config)) (*FileWatcher, error) {
	return NewFileWatcher(path, func() {
		log := logger.WithComponent("logging-watcher")
		lc, err := LoadLogging(path)
		if err != nil {
			log.Error().Err(err).Msg("Failed to reload logging configuration")
			return
		}
		if callback != nil {
			callback(lc)
		}
	})
}
