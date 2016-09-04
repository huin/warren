package nullmetric

import "github.com/prometheus/client_golang/prometheus"

type NoopCollector struct{}

var _ prometheus.Collector = NoopCollector{}

func (NoopCollector) Describe(chan<- *prometheus.Desc) {}
func (NoopCollector) Collect(chan<- prometheus.Metric) {}
