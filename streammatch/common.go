package streammatch

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/ActiveState/tail"
	promm "github.com/prometheus/client_golang/prometheus"
)

type VarCfg struct {
	promm.CounterOpts
	LabelNames []string
	Match      []MatchCfg
}

type MatchCfg struct {
	Pattern     string
	LabelValues []string
}

type varMatcherSet []varMatcher

func newVarMatcherSet(varCfgs []VarCfg) (varMatcherSet, error) {
	if len(varCfgs) == 0 {
		return nil, errors.New("no vars declared")
	}
	vms := make(varMatcherSet, 0, len(varCfgs))
	for _, varCfg := range varCfgs {
		varMatcher, err := newVarMatcher(varCfg)
		if err != nil {
			return nil, err
		}
		vms = append(vms, varMatcher)
	}
	return vms, nil
}

func (vms varMatcherSet) matchLines(lines <-chan *tail.Line) {
	for line := range lines {
		for i := range vms {
			vm := &vms[i]
			for j := range vm.matchers {
				m := &vm.matchers[j]
				if m.re.MatchString(line.Text) {
					m.ctr.Inc()
				}
			}
		}
	}
}

func (vms varMatcherSet) Describe(ch chan<- *promm.Desc) {
	for i := range vms {
		vms[i].ctrVec.Describe(ch)
	}
}

func (vms varMatcherSet) Collect(ch chan<- promm.Metric) {
	for i := range vms {
		vms[i].ctrVec.Collect(ch)
	}
}

type varMatcher struct {
	ctrVec   *promm.CounterVec
	matchers []matcher
}

func newVarMatcher(varCfg VarCfg) (varMatcher, error) {
	if varCfg.Name == "" {
		return varMatcher{}, errors.New("missing/empty var name")
	}
	if varCfg.Help == "" {
		return varMatcher{}, fmt.Errorf("missing/empty help declared, var %q", varCfg.Name)
	}
	if len(varCfg.Match) == 0 {
		return varMatcher{}, fmt.Errorf("no match defined, var %q", varCfg.Name)
	}

	ctrVec := promm.NewCounterVec(varCfg.CounterOpts, varCfg.LabelNames)
	matchers := make([]matcher, 0, len(varCfg.Match))
	for _, matchCfg := range varCfg.Match {
		matcher, err := newMatcher(matchCfg, ctrVec, varCfg.LabelNames)
		if err != nil {
			return varMatcher{}, fmt.Errorf("%v, var %q", err, varCfg.Name)
		}
		matchers = append(matchers, matcher)
	}
	return varMatcher{ctrVec: ctrVec, matchers: matchers}, nil
}

type matcher struct {
	re  *regexp.Regexp
	ctr promm.Counter
}

func newMatcher(matchCfg MatchCfg, ctrVec *promm.CounterVec, labelNames []string) (matcher, error) {
	re, err := regexp.Compile(matchCfg.Pattern)
	if err != nil {
		return matcher{}, err
	}
	ctr, err := ctrVec.GetMetricWithLabelValues(matchCfg.LabelValues...)
	if err != nil {
		return matcher{}, fmt.Errorf("%v for labels %v=%v",
			err, labelNames, matchCfg.LabelValues)
	}
	return matcher{re: re, ctr: ctr}, nil
}
