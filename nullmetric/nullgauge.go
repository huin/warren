package nullmetric

import "github.com/prometheus/client_golang/prometheus"

type GaugeVec interface {
	CommonBaseVec

	GetMetricWith(labels prometheus.Labels) (prometheus.Gauge, error)
	GetMetricWithLabelValues(lvs ...string) (prometheus.Gauge, error)
	With(labels prometheus.Labels) prometheus.Gauge
	WithLabelValues(lvs ...string) prometheus.Gauge
}

type NoopGaugeVec struct {
	NoopCollector
	CommonNoopMetricVec
}

var _ GaugeVec = NoopGaugeVec{}

func (NoopGaugeVec) GetMetricWith(labels prometheus.Labels) (prometheus.Gauge, error) {
	return NoopGauge{}, nil
}
func (NoopGaugeVec) GetMetricWithLabelValues(lvs ...string) (prometheus.Gauge, error) {
	return NoopGauge{}, nil
}
func (NoopGaugeVec) With(labels prometheus.Labels) prometheus.Gauge { return NoopGauge{} }
func (NoopGaugeVec) WithLabelValues(lvs ...string) prometheus.Gauge { return NoopGauge{} }

type NoopGauge struct {
	NoopMetric
	NoopCollector
}

var _ prometheus.Gauge = NoopGauge{}

func (NoopGauge) Set(float64)       {}
func (NoopGauge) Inc()              {}
func (NoopGauge) Dec()              {}
func (NoopGauge) Add(float64)       {}
func (NoopGauge) Sub(float64)       {}
func (NoopGauge) SetToCurrentTime() {}
