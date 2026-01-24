package adapter

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"sync"
)

// KiroAdapter implements the Adapter interface for Kiro CLI
type KiroAdapter struct {
	cmd            *exec.Cmd
	stdin          io.WriteCloser
	running        bool
	outputCallback func(OutputEvent)
	permCallback   func(PermissionRequest) PermissionResponse
	exitCallback   func(int)
	mu             sync.Mutex
}

func NewKiroAdapter() *KiroAdapter {
	return &KiroAdapter{}
}

func (a *KiroAdapter) Name() string {
	return "kiro"
}

func (a *KiroAdapter) DisplayName() string {
	return "Kiro CLI"
}

func (a *KiroAdapter) IsInstalled() bool {
	_, err := exec.LookPath("kiro")
	return err == nil
}

func (a *KiroAdapter) Start(workDir string, args []string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	cmdArgs := append([]string{"--headless"}, args...)
	a.cmd = exec.Command("kiro", cmdArgs...)
	a.cmd.Dir = workDir
	a.cmd.Env = append(os.Environ(), "KIRO_HOOKS_ENABLED=true")

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

	// Read stdout
	go a.readOutput(stdout, "stdout")
	go a.readOutput(stderr, "stderr")

	// Wait for exit
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

func (a *KiroAdapter) readOutput(r io.Reader, outputType string) {
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

func (a *KiroAdapter) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.cmd != nil && a.cmd.Process != nil {
		return a.cmd.Process.Kill()
	}
	return nil
}

func (a *KiroAdapter) IsRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.running
}

func (a *KiroAdapter) Send(input string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.stdin == nil {
		return nil
	}

	_, err := a.stdin.Write([]byte(input + "\n"))
	return err
}

func (a *KiroAdapter) OnOutput(callback func(OutputEvent)) {
	a.outputCallback = callback
}

func (a *KiroAdapter) OnPermission(callback func(PermissionRequest) PermissionResponse) {
	a.permCallback = callback
}

func (a *KiroAdapter) OnExit(callback func(int)) {
	a.exitCallback = callback
}
