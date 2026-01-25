package watcher

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

type EventType string

const (
	EventCreate EventType = "create"
	EventWrite  EventType = "write"
	EventMove   EventType = "move"
	EventDelete EventType = "delete"
)

type FileEvent struct {
	Type EventType
	Path string
}

type Handler interface {
	HandleFileEvent(event FileEvent) error
	IsMediaFile(path string) bool
}

type Watcher struct {
	fsWatcher *fsnotify.Watcher
	handler   Handler
	dryRun    bool
	recursive bool
}

type Option func(*Watcher)

func WithRecursive(recursive bool) Option {
	return func(w *Watcher) {
		w.recursive = recursive
	}
}

func NewWatcher(handler Handler, dryRun bool, opts ...Option) (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("unable to create watcher: %w", err)
	}

	w := &Watcher{
		fsWatcher: fsWatcher,
		handler:   handler,
		dryRun:    dryRun,
		recursive: true,
	}

	for _, opt := range opts {
		opt(w)
	}

	return w, nil
}

func (w *Watcher) Watch(paths []string) error {
	for _, path := range paths {
		if w.recursive {
			if err := w.addRecursive(path); err != nil {
				return err
			}
		} else {
			if err := w.fsWatcher.Add(path); err != nil {
				return fmt.Errorf("unable to watch %s: %w", path, err)
			}
			log.Printf("Watching: %s", path)
		}
	}
	return nil
}

func (w *Watcher) addRecursive(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		if strings.HasPrefix(filepath.Base(path), ".") {
			return filepath.SkipDir
		}
		if err := w.fsWatcher.Add(path); err != nil {
			return fmt.Errorf("unable to watch %s: %w", path, err)
		}
		log.Printf("Watching: %s", path)
		return nil
	})
}

func (w *Watcher) Start() error {
	log.Println("Jellywatch started. Press Ctrl+C to stop.")

	for {
		select {
		case event, ok := <-w.fsWatcher.Events:
			if !ok {
				return fmt.Errorf("watcher events channel closed")
			}

			if event.Op&fsnotify.Create == fsnotify.Create {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					if w.recursive && !strings.HasPrefix(filepath.Base(event.Name), ".") {
						w.fsWatcher.Add(event.Name)
						log.Printf("Now watching new directory: %s", event.Name)
					}
					continue
				}
			}

			if err := w.handleEvent(event); err != nil {
				log.Printf("Error handling event: %v", err)
			}

		case err, ok := <-w.fsWatcher.Errors:
			if !ok {
				return fmt.Errorf("watcher errors channel closed")
			}
			log.Printf("Watcher error: %v", err)
		}
	}
}

func (w *Watcher) Close() error {
	return w.fsWatcher.Close()
}

func (w *Watcher) handleEvent(event fsnotify.Event) error {
	if !w.isVideoFile(event.Name) {
		return nil
	}

	eventType := EventCreate
	if event.Op&fsnotify.Write == fsnotify.Write {
		eventType = EventWrite
	} else if event.Op&fsnotify.Rename == fsnotify.Rename {
		eventType = EventMove
	} else if event.Op&fsnotify.Remove == fsnotify.Remove {
		eventType = EventDelete
	}

	fileEvent := FileEvent{
		Type: eventType,
		Path: event.Name,
	}

	log.Printf("Event: %s - %s", eventType, filepath.Base(event.Name))

	return w.handler.HandleFileEvent(fileEvent)
}

func (w *Watcher) isVideoFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	videoExts := map[string]bool{
		".mkv": true, ".mp4": true, ".avi": true, ".mov": true,
		".wmv": true, ".flv": true, ".webm": true, ".m4v": true,
		".mpg": true, ".mpeg": true, ".m2ts": true, ".ts": true,
	}
	return videoExts[ext]
}
