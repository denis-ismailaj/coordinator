# Coordinator

## Description

Coordinator is a locking package that utilizes the local filesystem.

## How it works

The main concept of Coordinator is a _wait file_. Each lock contender creates a wait file
in order to get a place in line for the lock. The lock contender holds the lock when its
wait file is the first in the sorted list of files in the specified directory. When other wait files
exist before this lock contender, then it waits for the one directly preceding it to be removed.
By having contenders wait on only the contender before them, we avoid the thundering herd problem.

This idea was inspired by the Zookeeper paper.

## Usage

### Simple usage (no cutting in line)

    // Create a coordinator instance
	coordinator := coordination.Coordinator{
		Dir: path.Join(os.TempDir(), "example"),
	}

	// Create a wait file for this operator
	file, err := coordinator.CreateWaitFile()
	if err != nil {
		log.Fatal(err)
	}
    // Clean up when finished
	defer os.Remove(file.Name())

    // WaitInLine blocks until the lock contender is the first in line.
    coordinator.WaitInLine(context.Background())

### Usage with cutting in line

    // Create a coordinator instance
	coordinator := coordination.Coordinator{
		Dir: path.Join(os.TempDir(), "example"),
	}

	// Create a wait file for this operator
	file, err := coordinator.CreateWaitFile()
	if err != nil {
		log.Fatal(err)
	}
    // Clean up when finished
	defer os.Remove(file.Name())

	// Check if another contender has cut in line before us.
	ownFileChan := make(chan error)
	ownWatcher := coordinator.WaitForFile(coordinator.FilePath, ownFileChan)
	defer ownWatcher.Close()
	go func() {
		<-ownFileChan
		log.Fatal("The wait file of this queuer was forcibly removed. Quitting.")
	}()

    // Choose to wait:
    //
    // coordinator.WaitInLine(context.Background())
    //
    // or to cut in line:
    //
    // err := coordinator.CutInLine()
    // if err != nil {
    //     log.Fatal(err)
    // }


## Room for improvement

Coordinator has a weak point of not being able to deal with contenders that aren't able to do
cleanup (e.g. if they're `SIGKILL`-ed). This could be solved by having timeouts for wait files.
The owner of a wait file could `touch` the file at a certain interval. If it failed to do that for some
time, then the succeeding contender would be allowed to remove the stale wait file.
