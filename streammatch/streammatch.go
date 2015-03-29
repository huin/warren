package streammatch

import (
	"errors"
	"fmt"
	"os"

	"github.com/ActiveState/tail"
)

type FileCfg struct {
	File string
	Var  []VarCfg
}

// FileCollector implements prometheus.Collector for counter metrics based on
// matched line counters.
type FileCollector struct {
	// We don't export varMatcherSet type directly to avoid API clients relying
	// on its slice properties. We do expose Describe and Collect, though.
	varMatcherSet
}

func NewFileCollector(cfg FileCfg) (*FileCollector, error) {
	vms, err := newVarMatcherSet(cfg.Var)
	if err != nil {
		return nil, fmt.Errorf("%v, file %q", err, cfg.File)
	}

	if cfg.File == "" {
		return nil, errors.New("missing File name in FileCollector config")
	}
	lines, err := newFileTailChannel(cfg.File)
	if err != nil {
		return nil, err
	}

	go vms.matchLines(lines)

	return &FileCollector{vms}, nil
}

func newFileTailChannel(filename string) (<-chan *tail.Line, error) {
	tailFile, err := tail.TailFile(filename, tail.Config{
		Location:    &tail.SeekInfo{0, os.SEEK_END},
		ReOpen:      true,
		MustExist:   false,
		Follow:      true,
		MaxLineSize: 4096,
	})
	if err != nil {
		return nil, err
	}
	return tailFile.Lines, nil
}
