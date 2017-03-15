package linux

import (
	"fmt"
	"io/ioutil"
	"log"
	"strconv"
	"strings"

	"github.com/huin/warren/util"
	promm "github.com/prometheus/client_golang/prometheus"
)

/*
#include <unistd.h>
*/
import "C"

const (
	cpuStatsPath  = "/proc/stat"
	intFileEnding = "\x00\n"
)

var (
	// Named CPU states for each numeric CPU column in /proc/stat. The order
	// corresponds to the column order.
	cpuStates = []string{"user", "nice", "system", "idle", "iowait", "irq", "softirq", "steal", "guest", "guest_nice"}
)

type CpuConfig struct {
	// CPU states to export values for. See the (private) cpuStates variable for
	// allowed values.
	States []string
	// Export combined CPU cores' states only if true. Export individual CPU states if
	// false.
	Combined bool
	// Export individual CPU cores' states.
	ByCore bool
}

type cpuCollector struct {
	metrics      util.MetricCollection
	combinedTime *promm.GaugeVec
	byCoreTime   *promm.GaugeVec
	// Kernel jiffy time in seconds (typically 1/100 or something).
	jiffiesScaler float64
	// CPU metrics:
	// bitfield, corresponds to states in cpuStates, generated from
	// CpuConfig.States.
	recordedStates uint32

	// reuseable map for stating cpu/state labels. closely tied to readStats and
	// exportValues.
	metricLabels promm.Labels
}

func newCpuCollector(cfg CpuConfig, labels promm.Labels) (*cpuCollector, error) {
	cc := &cpuCollector{
		jiffiesScaler: 1 / float64(C.sysconf(C._SC_CLK_TCK)),
		metricLabels:  make(promm.Labels),
	}

	// Geneate recordedStates bitfield value.
	for _, state := range cfg.States {
		found := false
		for i, knownState := range cpuStates {
			if knownState == state {
				cc.recordedStates |= 1 << uint(i)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown CPU state %q, accepted values: %s",
				state, strings.Join(cpuStates, ", "))
		}
	}

	if cfg.Combined {
		cc.combinedTime = cc.metrics.NewGaugeVec(
			promm.GaugeOpts{
				Namespace: namespace, Name: "cpu_combined_seconds",
				Help:        "CPU time spent in various states, combined cores (seconds).",
				ConstLabels: labels,
			},
			[]string{"state"},
		)
	}

	if cfg.ByCore {
		cc.byCoreTime = cc.metrics.NewGaugeVec(
			promm.GaugeOpts{
				Namespace: namespace, Name: "cpu_by_core_seconds",
				Help:        "CPU time spent in various states, per core (seconds).",
				ConstLabels: labels,
			},
			[]string{"core", "state"},
		)
	}

	return cc, nil
}

func (cc *cpuCollector) Describe(ch chan<- *promm.Desc) {
	cc.metrics.Describe(ch)
}

func (cc *cpuCollector) Collect(ch chan<- promm.Metric) {
	if err := cc.readStats(); err != nil {
		log.Printf("Error reading CPU stats: %v", err)
	}
	cc.metrics.Collect(ch)
}

func (cc *cpuCollector) readStats() error {
	data, err := ioutil.ReadFile(cpuStatsPath)
	if err != nil {
		return err
	}
	s := string(data)
	for _, l := range strings.Split(s, "\n") {
		if !strings.HasPrefix(l, "cpu") {
			// "cpu" lines are all at the start, skip the remainder.
			break
		}
		values := strings.Fields(l)
		if values[0] == "cpu" {
			if cc.combinedTime != nil {
				delete(cc.metricLabels, "core")
				cc.exportValues(values[1:], cc.combinedTime)
			}
		} else if cc.byCoreTime != nil {
			cc.metricLabels["core"] = values[0]
			cc.exportValues(values[1:], cc.byCoreTime)
		}
	}
	return nil
}

// values should be a "cpu"-prefixed set of values from /proc/stat.
// cc.metricLabels must have "core" label set or removed as appropriate for cv.
func (cc *cpuCollector) exportValues(values []string, cv *promm.GaugeVec) {
	for stateIndex, valueStr := range values {
		if stateIndex > len(cpuStates) {
			break
		}
		if cc.recordedStates&(1<<uint(stateIndex)) != 0 {
			jiffies, err := strconv.ParseUint(valueStr, 10, 64)
			if err != nil {
				continue
			}
			cc.metricLabels["state"] = cpuStates[stateIndex]
			cv.With(cc.metricLabels).Set(float64(jiffies) * cc.jiffiesScaler)
		}
	}
}
