package stack

import (
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"time"
)

// Runner runs an external command.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

type execRunner struct {
	logger *log.Logger
}

// NewExecRunner returns a Runner for the current working directory.
func NewExecRunner(debug io.Writer) Runner {
	return execRunner{logger: log.New(debug, "", 0)}
}

func (r execRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	started := time.Now()
	r.logger.Printf("exec %s %q", name, args)
	out, err := exec.CommandContext(ctx, name, args...).Output()
	r.logger.Printf("done %s in %s", name, time.Since(started))
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			return "", fmt.Errorf("run %s: %w: %s", name, err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("run %s: %w", name, err)
	}
	return string(out), nil
}
