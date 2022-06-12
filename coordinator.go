package coordination

import (
	"context"
	"errors"
	"fmt"
	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"time"
)

// Coordinator is a locking package that utilizes the local filesystem.
// The main concept of Coordinator is a "wait file". Each lock contender creates a wait file
// in order to get a place in line for the lock. The lock contender holds the lock when its
// wait file is the first in the sorted list of files in the Dir directory. When other wait files
// exist before this lock contender, then it waits for the one directly preceding it to be removed.
// By having contenders wait on only the contender before them, we avoid the thundering herd problem.
type Coordinator struct {
	Dir      string
	FilePath string
}

// WaitForFile watches the file at filePath and waits for it to be removed.
// It writes nil to the channel when the file is removed or an error.
func (co *Coordinator) WaitForFile(filePath string, channel chan error) *fsnotify.Watcher {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		channel <- err
		return watcher
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if event.Name != filePath {
					continue
				}
				if !ok {
					channel <- errors.New("fsnotify channel closed abruptly")
				}
				if event.Op&fsnotify.Remove == fsnotify.Remove {
					channel <- nil
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					channel <- err
				}
			}
		}
	}()

	// When using kqueue you can receive REMOVE events by watching
	// the removed file itself, but inotify doesn't seem to work that
	// way, so when running on Linux I'm watching the parent dir instead.
	if runtime.GOOS == "linux" {
		err = watcher.Add(filepath.Dir(filePath))
	} else {
		err = watcher.Add(filePath)
	}
	if err != nil {
		channel <- err
	}

	return watcher
}

// CreateWaitFile creates a file which is used by a lock contender to hold a place in line for the lock.
// Each file name has a timestamp of when it was created and an additional random suffix to avoid races.
func (co *Coordinator) CreateWaitFile() (*os.File, error) {
	namePattern := fmt.Sprintf("queuer-%d-*", time.Now().UnixNano())
	err := os.MkdirAll(co.Dir, os.ModePerm)
	if err != nil {
		return nil, err
	}

	file, err := ioutil.TempFile(co.Dir, namePattern)
	if err != nil {
		return nil, err
	}
	co.FilePath = file.Name()

	return file, nil
}

// WaitInLine blocks until the lock contender is the first in line.
func (co *Coordinator) WaitInLine(ctx context.Context) {
	for {
		files, err := os.ReadDir(co.Dir)
		if err != nil {
			log.Fatal(err)
		}

		var toWatch string

		for i, f := range files {
			if path.Join(co.Dir, f.Name()) != co.FilePath {
				continue
			}
			if i == 0 {
				log.Info("First in line.")
				return
			}

			toWatch = path.Join(co.Dir, files[i-1].Name())
		}

		log.Infof("Waiting for queuer with file %s to exit.", toWatch)

		watchChan := make(chan error)
		watcher := co.WaitForFile(toWatch, watchChan)

		select {
		case err := <-watchChan:
			if err != nil {
				log.Fatal(err)
			}
			break
		case <-ctx.Done():
			return
		}

		watcher.Close()
	}
}

// CutInLine forcibly removes the current lock holder and preceding lock contenders
// and makes the current contender acquire the lock.
// Note that this does not affect contenders that succeed the current contender in the line.
func (co *Coordinator) CutInLine() error {
	files, err := os.ReadDir(co.Dir)
	if err != nil {
		return err
	}

	for _, f := range files {
		currentFileName := path.Join(co.Dir, f.Name())
		if currentFileName == co.FilePath {
			break
		}
		err := os.Remove(currentFileName)
		if err != nil {
			return err
		}
	}

	return nil
}
