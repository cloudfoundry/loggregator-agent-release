package collector

import (
	"os"
	"strconv"
	"syscall"
)

type ConfigApplier struct {
	pidFile string
}

func NewConfigApplier(pidFile string) *ConfigApplier {
	return &ConfigApplier{pidFile: pidFile}
}

func (a *ConfigApplier) Apply() error {
	b, err := os.ReadFile(a.pidFile)
	if err != nil {
		return err
	}

	pid, err := strconv.Atoi(string(b))
	if err != nil {
		return err
	}

	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	p.Signal(syscall.SIGHUP)
	if err != nil {
		return err
	}
	return nil
}
