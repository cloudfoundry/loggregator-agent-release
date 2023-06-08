package collector

import (
	"io"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

type Runner struct {
	pidFile string
	cmd     string
	args    []string
	stdout  io.Writer
	stderr  io.Writer
	sd      time.Duration
	log     *logrus.Logger
}

func NewRunner(pidFile string, cmd string, args []string, stdout io.Writer, stderr io.Writer, sd time.Duration, log *logrus.Logger) *Runner {
	return &Runner{
		pidFile: pidFile,
		cmd:     cmd,
		stdout:  stdout,
		stderr:  stderr,
		args:    args,
		sd:      sd,
		log:     log,
	}
}

func (r *Runner) RemovePidFile() {
	_ = os.Remove(r.pidFile)
}

func (r *Runner) IsRunning() bool {
	p, err := r.process()
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	if err != nil {
		return false
	}
	return true
}

func (r *Runner) Start() error {
	c := exec.Command(r.cmd, r.args...) // #nosec G204
	c.Stdout = r.stdout
	c.Stderr = r.stderr

	if err := c.Start(); err != nil {
		return err
	}

	if err := os.WriteFile(r.pidFile, []byte(strconv.Itoa(c.Process.Pid)), 0600); err != nil {
		return err
	}

	go func() {
		// XXX: Need to consume process output too
		if err := c.Wait(); err != nil {
			r.log.WithError(err).Error("process exited with an error")
		}
	}()

	return nil
}

func (r *Runner) Stop() error {
	p, err := r.process()
	if err != nil {
		return err
	}

	timeout := time.NewTimer(r.sd)
	rt := time.NewTimer(10 * time.Millisecond)
	defer timeout.Stop()
	defer rt.Stop()

	for {
		select {
		case <-rt.C:
			_ = p.Signal(syscall.SIGTERM)
			if !r.IsRunning() {
				return nil
			}
		case <-timeout.C:
			_ = p.Kill()
			return nil
		}
	}
}

func (r *Runner) process() (*os.Process, error) {
	b, err := os.ReadFile(r.pidFile)
	if err != nil {
		return nil, err
	}

	pid, err := strconv.Atoi(string(b))
	if err != nil {
		return nil, err
	}

	p, err := os.FindProcess(pid)
	if err != nil {
		return nil, err
	}
	return p, nil
}
