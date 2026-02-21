package adapter

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"sync"
)

// ClineAdapter implements the Adapter interface for Cline CLI
type ClineAdapter struct {
	cmd            *exec.Cmd
	stdin          io.WriteCloser
	running        bool
	outputCallback func(OutputEvent)
	permCallback   func(PermissionRequest) PermissionResponse
	exitCallback   func(int)
	mu             sync.Mutex
}

func NewClineAdapter() *ClineAdapter {
	return &ClineAdapter{}
}

func (a *ClineAdapter) Name() string {
	return "cline"
}

func (a *ClineAdapter) DisplayName() string {
	return "Cline CLI"
}

func (a *ClineAdapter) IsInstalled() bool {
	_, err := exec.LookPath("cline")
	return err == nil
}

func (a *ClineAdapter) Start(workDir string, args []string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Cline uses -s for settings
	cmdArgs := append([]string{"-s", "hooks_enabled=true"}, args...)
	a.cmd = exec.Command("cline", cmdArgs...)
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

func (a *ClineAdapter) readOutput(r io.Reader, outputType string) {
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

func (a *ClineAdapter) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.cmd != nil && a.cmd.Process != nil {
		return a.cmd.Process.Kill()
	}
	return nil
}

func (a *ClineAdapter) IsRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.running
}

func (a *ClineAdapter) Send(input string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.stdin == nil {
		return nil
	}

	_, err := a.stdin.Write([]byte(input + "\n"))
	return err
}

func (a *ClineAdapter) OnOutput(callback func(OutputEvent)) {
	a.outputCallback = callback
}

func (a *ClineAdapter) OnPermission(callback func(PermissionRequest) PermissionResponse) {
	a.permCallback = callback
}

func (a *ClineAdapter) OnExit(callback func(int)) {
	a.exitCallback = callback
}

func (a *ClineAdapter) Resize(cols, rows int) error {
	// TODO: Implement PTY resize for Cline
	return nil
}

func (a *ClineAdapter) StartWithSize(workDir string, args []string, cols, rows int) error {
	// Cline uses stdin/stdout, not PTY, so size is ignored
	return a.Start(workDir, args)
}
