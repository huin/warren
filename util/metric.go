package util

import (
	promm "github.com/prometheus/client_golang/prometheus"
)

type MetricCollection []promm.Collector

func (mc *MetricCollection) Add(c promm.Collector) {
	*mc = append(*mc, c)
}

func (mc *MetricCollection) NewCounter(opts promm.CounterOpts) promm.Counter {
	c := promm.NewCounter(opts)
	mc.Add(c)
	return c
}

func (mc *MetricCollection) NewCounterVec(opts promm.CounterOpts, labelNames []string) *promm.CounterVec {
	c := promm.NewCounterVec(opts, labelNames)
	mc.Add(c)
	return c
}

func (mc *MetricCollection) NewGauge(opts promm.GaugeOpts) promm.Gauge {
	c := promm.NewGauge(opts)
	mc.Add(c)
	return c
}

func (mc *MetricCollection) NewGaugeVec(opts promm.GaugeOpts, labelNames []string) *promm.GaugeVec {
	c := promm.NewGaugeVec(opts, labelNames)
	mc.Add(c)
	return c
}

func (mc MetricCollection) Describe(ch chan<- *promm.Desc) {
	for _, c := range mc {
		c.Describe(ch)
	}
}

func (mc MetricCollection) Collect(ch chan<- promm.Metric) {
	for _, c := range mc {
		c.Collect(ch)
	}
}
