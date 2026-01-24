package adapter

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"sync"
)

// BaseAdapter provides common functionality for CLI adapters
type BaseAdapter struct {
	name           string
	displayName    string
	command        string
	cmd            *exec.Cmd
	stdin          io.WriteCloser
	running        bool
	outputCallback func(OutputEvent)
	permCallback   func(PermissionRequest) PermissionResponse
	exitCallback   func(int)
	mu             sync.Mutex
}

func (a *BaseAdapter) IsInstalled() bool {
	_, err := exec.LookPath(a.command)
	return err == nil
}

func (a *BaseAdapter) Start(workDir string, args []string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.cmd = exec.Command(a.command, args...)
	a.cmd.Dir = workDir
	a.cmd.Env = os.Environ()

	var err error
	a.stdin, err = a.cmd.StdinPipe()
	if err != nil {
		return err
	}

	stdout, err := a.cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := a.cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := a.cmd.Start(); err != nil {
		return err
	}

	a.running = true

	go a.readOutput(stdout, "stdout")
	go a.readOutput(stderr, "stderr")

	go func() {
		err := a.cmd.Wait()
		a.mu.Lock()
		a.running = false
		a.mu.Unlock()

		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			}
		}

		if a.exitCallback != nil {
			a.exitCallback(exitCode)
		}
	}()

	return nil
}

func (a *BaseAdapter) readOutput(r io.Reader, outputType string) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if a.outputCallback != nil {
			a.outputCallback(OutputEvent{
				Type:    outputType,
				Content: scanner.Text(),
			})
		}
	}
}

func (a *BaseAdapter) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.cmd != nil && a.cmd.Process != nil {
		return a.cmd.Process.Kill()
	}
	return nil
}

func (a *BaseAdapter) IsRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.running
}

func (a *BaseAdapter) Send(input string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.stdin == nil {
		return nil
	}

	_, err := a.stdin.Write([]byte(input + "\n"))
	return err
}

func (a *BaseAdapter) OnOutput(callback func(OutputEvent)) {
	a.outputCallback = callback
}

func (a *BaseAdapter) OnPermission(callback func(PermissionRequest) PermissionResponse) {
	a.permCallback = callback
}

func (a *BaseAdapter) OnExit(callback func(int)) {
	a.exitCallback = callback
}
