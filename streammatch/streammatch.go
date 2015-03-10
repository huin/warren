package streammatch

import (
	"errors"
	"fmt"
	"os"
	"regexp"

	"github.com/ActiveState/tail"
	promm "github.com/prometheus/client_golang/prometheus"
)

type FileCfg struct {
	File string
	Var  []VarCfg
}

type VarCfg struct {
	promm.CounterOpts
	LabelNames []string
	Match      []MatchCfg
}

type MatchCfg struct {
	Pattern     string
	LabelValues []string
}

type FileCollector struct {
	tailFile    *tail.Tail
	varMatchers []varMatcher
}

type varMatcher struct {
	ctrVec   *promm.CounterVec
	matchers []matcher
}

type matcher struct {
	re  *regexp.Regexp
	ctr promm.Counter
}

func NewFileCollector(cfg FileCfg) (*FileCollector, error) {
	if len(cfg.Var) == 0 {
		return nil, fmt.Errorf("no vars declared for file %q", cfg.File)
	}

	varMatchers := make([]varMatcher, 0, len(cfg.Var))
	for _, varCfg := range cfg.Var {
		if varCfg.Name == "" {
			return nil, fmt.Errorf("missing/empty var name declared for file %q", cfg.File)
		}
		if varCfg.Help == "" {
			return nil, fmt.Errorf("missing/empty help declared for file %q, var %q", cfg.File, varCfg.Name)
		}
		if len(varCfg.Match) == 0 {
			return nil, fmt.Errorf("no match defined for file %q, var %q", cfg.File, varCfg.Name)
		}

		ctrVec := promm.NewCounterVec(varCfg.CounterOpts, varCfg.LabelNames)
		matchers := make([]matcher, 0, len(varCfg.Match))
		for _, matchCfg := range varCfg.Match {
			re, err := regexp.Compile(matchCfg.Pattern)
			if err != nil {
				return nil, err
			}
			ctr, err := ctrVec.GetMetricWithLabelValues(matchCfg.LabelValues...)
			if err != nil {
				return nil, fmt.Errorf("%v for labels %v=%v, file %q, var %q",
					err, varCfg.LabelNames, matchCfg.LabelValues, cfg.File, varCfg.Name)
			}
			matchers = append(matchers, matcher{re: re, ctr: ctr})
		}

		varMatchers = append(varMatchers, varMatcher{
			ctrVec:   ctrVec,
			matchers: matchers,
		})
	}

	if cfg.File == "" {
		return nil, errors.New("missing File name in FileCollector config")
	}
	tailFile, err := tail.TailFile(cfg.File, tail.Config{
		Location:    &tail.SeekInfo{0, os.SEEK_END},
		ReOpen:      true,
		MustExist:   false,
		Follow:      true,
		MaxLineSize: 4096,
	})
	if err != nil {
		return nil, err
	}

	fc := &FileCollector{
		tailFile:    tailFile,
		varMatchers: varMatchers,
	}

	go fc.watcher()

	return fc, nil
}

func (fc *FileCollector) watcher() {
	for line := range fc.tailFile.Lines {
		for i := range fc.varMatchers {
			vm := &fc.varMatchers[i]
			for j := range vm.matchers {
				m := &vm.matchers[j]
				if m.re.MatchString(line.Text) {
					m.ctr.Inc()
				}
			}
		}
	}
}

func (fc *FileCollector) Describe(ch chan<- *promm.Desc) {
	for i := range fc.varMatchers {
		fc.varMatchers[i].ctrVec.Describe(ch)
	}
}

func (fc *FileCollector) Collect(ch chan<- promm.Metric) {
	for i := range fc.varMatchers {
		fc.varMatchers[i].ctrVec.Collect(ch)
	}
}
