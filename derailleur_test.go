package derailleur

import (
	"context"
	log "github.com/sirupsen/logrus"
	"os"
	"path"
	"testing"
	"time"
)

func TestWaitForFile(t *testing.T) {
	derailleur := Derailleur{}

	temp, _ := os.CreateTemp(os.TempDir(), "test-*")

	fileChan := make(chan error)
	watcher := derailleur.WaitForFile(temp.Name(), fileChan)
	defer watcher.Close()

	deleted := false

	go func() {
		time.Sleep(2 * time.Second)
		_ = os.Remove(temp.Name())
		deleted = true
	}()

	select {
	case <-time.After(5 * time.Second):
		t.Fatal("Didn't react to file being removed.")
	case <-fileChan:
		if !deleted {
			t.Fatal("Watcher activity before deleting.")
		}
	}
}

func TestWaitInLine(t *testing.T) {
	dir, err := os.MkdirTemp("", "juju-task-testing-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(dir)

	derailleur := Derailleur{
		Dir: dir,
	}

	file, err := derailleur.CreateWaitFile()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(file.Name())

	first, _ := os.Create(path.Join(dir, "0"))
	defer os.Remove(first.Name())

	done := make(chan struct{})

	go func() {
		derailleur.WaitInLine(context.Background())
		done <- struct{}{}
	}()

	deleted := false

	go func() {
		time.Sleep(2 * time.Second)
		_ = os.Remove(first.Name())
		deleted = true
	}()

	select {
	case <-time.After(5 * time.Second):
		t.Fatal("Didn't react to file being removed.")
	case <-done:
		if !deleted {
			t.Fatal("Watcher activity before deleting.")
		}
	}
}

func TestWaitInLineMultiple(t *testing.T) {
	dir, err := os.MkdirTemp("", "juju-task-testing-*")
	if err != nil {
		t.Fatal(err)
	}

	n := 5
	done := make(chan string)

	for i := 0; i < n; i++ {
		derailleur := Derailleur{
			Dir: dir,
		}
		file, err := derailleur.CreateWaitFile()
		if err != nil {
			t.Fatal(err)
		}
		//goland:noinspection GoDeferInLoop
		defer os.Remove(file.Name())

		go func() {
			derailleur.WaitInLine(context.Background())
			done <- derailleur.FilePath
		}()
	}

	files, _ := os.ReadDir(dir)

	// First contender should wake up immediately
	select {
	case <-time.After(2 * time.Second):
		t.Fatal("Queuer not waking up.")
	case c := <-done:
		if c != path.Join(dir, files[0].Name()) {
			t.Fatal("Wrong wait order.")
		}
	}

	for i := 1; i < n; i++ {
		queuer := path.Join(dir, files[i].Name())
		deleted := false

		go func(i int) {
			time.Sleep(2 * time.Second)
			toRemove := path.Join(dir, files[i-1].Name())
			log.Printf("removing %s", toRemove)
			log.Printf("expecting to wake up %s", queuer)
			err = os.Remove(toRemove)
			if err != nil {
				log.Error(err)
			}
			deleted = true
		}(i)

		select {
		case <-time.After(5 * time.Second):
			t.Fatal("Queuer not waking up.")
		case c := <-done:
			if !deleted {
				t.Fatal("Cut in line.")
			}
			if c != queuer {
				t.Fatal("Wrong wait order.")
			}
		}
	}
}

func TestWaitInLineCancel(t *testing.T) {
	dir, err := os.MkdirTemp("", "juju-task-testing-*")
	if err != nil {
		t.Fatal(err)
	}

	derailleur := Derailleur{
		Dir: dir,
	}

	file, err := derailleur.CreateWaitFile()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(file.Name())

	first, _ := os.Create(path.Join(dir, "0"))
	defer os.Remove(first.Name())

	done := make(chan struct{})

	ctx, cancelFn := context.WithCancel(context.Background())

	go func() {
		derailleur.WaitInLine(ctx)
		done <- struct{}{}
	}()

	cancelFn()

	select {
	case <-time.After(1 * time.Second):
		t.Fatal("WaitInLine not cancelling")
	case <-done:
	}
}

func TestCutInLine(t *testing.T) {
	dir, err := os.MkdirTemp("", "juju-task-testing-*")
	if err != nil {
		t.Fatal(err)
	}

	n := 10
	for i := 0; i < n; i++ {
		derailleur := Derailleur{
			Dir: dir,
		}
		file, err := derailleur.CreateWaitFile()
		if err != nil {
			t.Fatal(err)
		}
		//goland:noinspection GoDeferInLoop
		defer os.Remove(file.Name())
	}

	cutter := Derailleur{
		Dir: dir,
	}
	file, err := cutter.CreateWaitFile()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(file.Name())

	err = cutter.CutInLine()
	if err != nil {
		t.Fatal(err)
	}

	files, _ := os.ReadDir(dir)

	if len(files) > 1 {
		t.Fatal("too many wait files found")
	}
}
