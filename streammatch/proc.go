package streammatch

import (
	"bufio"
	"errors"
	"log"
	"os"
	"time"

	"github.com/ActiveState/tail"
	"github.com/huin/warren/util"
	promm "github.com/prometheus/client_golang/prometheus"
)

// Configures matching output from a command's output.
type ProcCfg struct {
	// The command, first item being the executable name/path.
	Command []string
	// The CWD for the process.
	Dir string
	// If the process exits, wait this long before restarting. Defaults to 30
	// seconds if unspecified.
	RetryInterval util.Duration
	// Configurations for the variable matching on stdout and stderr.
	Stdout, Stderr []VarCfg
}

type ProcCollector struct {
	stdout varMatcherSet
	stderr varMatcherSet
	// TODO: Metrics for the collector itself: specifically process restart count.
}

func NewProcCollector(cfg ProcCfg) (*ProcCollector, error) {
	if len(cfg.Command) < 1 {
		return nil, errors.New("missing command in ProcCollector config")
	}
	if len(cfg.Stdout) == 0 && len(cfg.Stderr) == 0 {
		return nil, errors.New("must specify at least one stdout and/or stderr variable matching")
	}

	if cfg.RetryInterval.Duration == 0 {
		cfg.RetryInterval.Duration = 30 * time.Second
	}

	c := new(ProcCollector)

	var err error
	// The files passed to the child process.
	var childStdout, childStderr *os.File

	c.stdout, childStdout, err = newSubProcOutput(cfg.Stdout)
	if err != nil {
		return nil, err
	}
	c.stderr, childStderr, err = newSubProcOutput(cfg.Stderr)
	if err != nil {
		return nil, err
	}

	startProc := func() (*os.Process, error) {
		return os.StartProcess(cfg.Command[0], cfg.Command, &os.ProcAttr{
			Dir:   cfg.Dir,
			Files: []*os.File{nil, childStdout, childStderr},
		})
	}

	// Check that we can start the process at least.
	proc, err := startProc()
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			if state, err := proc.Wait(); err != nil {
				if err != nil {
					log.Println("ProcCollector could not wait on child process: ", err)
					// Not much we can do in this (unlikely) state. We don't want to keep
					// spawning processes we can't wait on.
					return
				} else {
					log.Printf("ProcCollector child process exited (restart in %v): %s",
						cfg.RetryInterval, state.String())
				}
			}
			time.Sleep(cfg.RetryInterval.Duration)
			proc, err = startProc()
		}
	}()

	return c, nil
}

func (pc *ProcCollector) Describe(ch chan<- *promm.Desc) {
	if pc.stdout != nil {
		pc.stdout.Describe(ch)
	}
	if pc.stderr != nil {
		pc.stderr.Describe(ch)
	}
}

func (pc *ProcCollector) Collect(ch chan<- promm.Metric) {
	if pc.stdout != nil {
		pc.stdout.Collect(ch)
	}
	if pc.stderr != nil {
		pc.stderr.Collect(ch)
	}
}

func newSubProcOutput(cfg []VarCfg) (varMatcherSet, *os.File, error) {
	if len(cfg) == 0 {
		return nil, nil, nil
	}
	vms, err := newVarMatcherSet(cfg)
	if err != nil {
		return nil, nil, err
	}
	reader, out, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	lines := make(chan *tail.Line)
	go func() {
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			lines <- &tail.Line{Text: string(scanner.Bytes())}
		}
		err := scanner.Err()
		lines <- &tail.Line{Err: err}
		log.Println("ProcCollector encountered error reading from process output: ", err)
	}()
	go vms.matchLines(lines)
	return vms, out, nil
}
