// Package nullmetric contains placeholder metrics for prometheus that do nothing.
package nullmetric

import (
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

type CommonBaseVec interface {
	prometheus.Collector

	Delete(labels prometheus.Labels) bool
	DeleteLabelValues(lvs ...string) bool
	Reset()
}

type CommonNoopMetricVec struct{}

func (CommonNoopMetricVec) Delete(labels prometheus.Labels) bool { return false }
func (CommonNoopMetricVec) DeleteLabelValues(lvs ...string) bool { return false }
func (CommonNoopMetricVec) Reset()                               {}

type NoopMetric struct{}

var _ prometheus.Metric = NoopMetric{}

func (NoopMetric) Desc() *prometheus.Desc  { return nil }
func (NoopMetric) Write(*dto.Metric) error { return nil }
