package httpexport

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/huin/warren/util"
	promm "github.com/prometheus/client_golang/prometheus"
)

type Config struct {
	HandlerPath string
	LabelNames  []string
	Counter     *promm.CounterOpts
	Gauge       *promm.GaugeOpts
	Histogram   *promm.HistogramOpts
}

type Collector struct {
	labelNames []string
	metrics    util.MetricCollection
	counter    *promm.CounterVec
	gauge      *promm.GaugeVec
	histo      *promm.HistogramVec
}

func New(cfg Config) (*Collector, error) {
	c := &Collector{}
	for _, ln := range cfg.LabelNames {
		if ln == "" {
			return nil, errors.New("got empty label name")
		}
		if strings.HasPrefix(ln, "_") {
			return nil, fmt.Errorf("got label name %q, leading underscores are reserved for internal use", ln)
		}
	}
	c.labelNames = cfg.LabelNames
	if cfg.Counter != nil {
		c.counter = c.metrics.NewCounterVec(*cfg.Counter, cfg.LabelNames)
	}
	if cfg.Gauge != nil {
		c.gauge = c.metrics.NewGaugeVec(*cfg.Gauge, cfg.LabelNames)
	}
	if cfg.Histogram != nil {
		c.histo = c.metrics.NewHistogramVec(*cfg.Histogram, cfg.LabelNames)
	}
	if len(c.metrics) == 0 {
		return nil, errors.New("at least one of Counter, Gauge, or Histogram must be set")
	}
	if cfg.HandlerPath == "" {
		return nil, errors.New("HandlerPath must be set")
	}
	http.HandleFunc(cfg.HandlerPath, c.handler)
	return c, nil
}

func (c *Collector) Describe(ch chan<- *promm.Desc) { c.metrics.Describe(ch) }

func (c *Collector) Collect(ch chan<- promm.Metric) { c.metrics.Collect(ch) }

func (c *Collector) handler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodPost, http.MethodPut:
		// Accepted methods.
		// Maybe one day allow configuration to specify which are allowed, but
		// seems okay to be broadly accepting for now.
	default:
		// Any other method is likely to be a mistake.
		http.Error(w, "Unhandled method: "+r.Method, http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Error parsing arguments: "+err.Error(), http.StatusBadRequest)
		return
	}

	lv := make([]string, 0, len(c.labelNames))
	for _, ln := range c.labelNames {
		lv = append(lv, r.Form.Get(ln))
	}

	var err error

	if c.counter != nil {
		v := 1.0
		if s := r.Form.Get("_add"); s != "" {
			if v, err = strconv.ParseFloat(s, 64); err != nil {
				http.Error(w, "Error parsing _add argument: "+err.Error(), http.StatusBadRequest)
				return
			}
		}
		c, err := c.counter.GetMetricWithLabelValues(lv...)
		if err != nil {
			log.Printf("Error getting httpexport counter metric with values %q: %v", lv, err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		c.Add(v)
	}

	if c.gauge != nil {
		s := r.Form.Get("_set")
		if s == "" {
			http.Error(w, "_set is required", http.StatusBadRequest)
			return
		}
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			http.Error(w, "Error parsing _set: "+err.Error(), http.StatusBadRequest)
			return
		}
		g, err := c.gauge.GetMetricWithLabelValues(lv...)
		if err != nil {
			log.Printf("Error getting httpexport gauge metric with values %q: %v", lv, err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		g.Set(v)
	}

	if c.histo != nil {
		s := r.Form.Get("_observe")
		if s == "" {
			http.Error(w, "_observe is required", http.StatusBadRequest)
			return
		}
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			http.Error(w, "Error parsing _observe: "+err.Error(), http.StatusBadRequest)
			return
		}
		h, err := c.histo.GetMetricWithLabelValues(lv...)
		if err != nil {
			log.Printf("Error getting httpexport histogram metric with values %q: %v", lv, err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		h.Observe(v)
	}

	http.Error(w, "ok", http.StatusOK)
}
