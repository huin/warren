package systemd

import (
	"fmt"
	"log"

	"github.com/coreos/go-systemd/dbus"
	"github.com/huin/warren/util"
	"github.com/prometheus/client_golang/prometheus"
)

type Config struct {
	// "dbus" or "direct", how to connect to systemd to query state.
	ConnType ConnType `toml:"conn_type"`
	// The constant labels to attach to the metrics.
	ConstLabels prometheus.Labels `toml:"const_labels"`
}

type ConnType int

const (
	ConnTypeDefault ConnType = iota
	ConnTypeDbus
	ConnTypeDirect
)

func (ct ConnType) String() string {
	switch ct {
	case ConnTypeDefault:
		return "default"
	case ConnTypeDbus:
		return "dbus"
	case ConnTypeDirect:
		return "direct"
	default:
		return fmt.Sprintf("ConnType(%d)", int(ct))
	}
}

func (ct *ConnType) UnmarshalText(text []byte) error {
	s := string(text)
	switch s {
	case "dbus":
		*ct = ConnTypeDbus
	case "direct":
		*ct = ConnTypeDirect
	default:
		return fmt.Errorf("unknown connection type: %q", s)
	}
	return nil
}

type Collector struct {
	conn    *dbus.Conn
	metrics util.MetricCollection
	units   map[string]*unitMetrics
	loaded  *prometheus.GaugeVec
	active  *prometheus.GaugeVec
}

func New(cfg Config) (*Collector, error) {
	var conn *dbus.Conn
	var err error
	switch cfg.ConnType {
	case ConnTypeDefault, ConnTypeDbus:
		conn, err = dbus.New()
	case ConnTypeDirect:
		conn, err = dbus.NewSystemdConnection()
	default:
		return nil, fmt.Errorf("unhandled ConnType: %v", cfg.ConnType)
	}
	if err != nil {
		return nil, fmt.Errorf("could not connect to systemd: %v", err)
	}

	var metrics util.MetricCollection
	c := &Collector{
		conn:    conn,
		metrics: nil,
		units:   make(map[string]*unitMetrics),
		loaded: metrics.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace:   "systemd",
				Subsystem:   "units",
				Name:        "loaded",
				Help:        "1 if the unit is loaded, 0 otherwise.",
				ConstLabels: cfg.ConstLabels,
			},
			[]string{"unit"},
		),
		active: metrics.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace:   "systemd",
				Subsystem:   "units",
				Name:        "active",
				Help:        "1 if the unit is active, 0 otherwise.",
				ConstLabels: cfg.ConstLabels,
			},
			[]string{"unit"},
		),
	}
	c.metrics = metrics
	return c, nil
}

func (c *Collector) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.metrics.Describe(ch)
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	for _, um := range c.units {
		um.seen = false
	}

	uss, err := c.conn.ListUnits()
	if err != nil {
		log.Print("Error getting systemd unit status: ", err)
	}
	// Update/create from found units.
	for i := range uss {
		us := &uss[i]
		um, ok := c.units[us.Name]
		if !ok {
			var err error
			um = &unitMetrics{}
			if um.loaded, err = c.loaded.GetMetricWithLabelValues(us.Name); err != nil {
				log.Printf("Error getting systemd loaded metric with label %q: %v", us.Name, err)
				continue
			}
			if um.active, err = c.active.GetMetricWithLabelValues(us.Name); err != nil {
				log.Printf("Error getting systemd active metric with label %q: %v", us.Name, err)
				continue
			}
			c.units[us.Name] = um
		}
		um.seen = true
		um.update(us)
	}

	// Clear out units that were not seen.
	for un, um := range c.units {
		if !um.seen {
			c.loaded.DeleteLabelValues(un)
			c.active.DeleteLabelValues(un)
			delete(c.units, un)
		}
	}

	c.metrics.Collect(ch)
}

type unitMetrics struct {
	seen   bool
	loaded prometheus.Gauge
	active prometheus.Gauge
}

func (um *unitMetrics) update(status *dbus.UnitStatus) {
	if status.LoadState == "loaded" {
		um.loaded.Set(1)
	} else {
		um.loaded.Set(0)
	}
	if status.ActiveState == "active" {
		um.active.Set(1)
	} else {
		um.active.Set(0)
	}
}
