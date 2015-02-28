package main

import (
	"strconv"

	"github.com/huin/gocc"
	promm "github.com/prometheus/client_golang/prometheus"
)

const (
	ccNamespace = "currentcost"
)

type CurrentCostConfig struct {
	Name   string
	Device string
}

type CurrentCostCollector struct {
	cfg         CurrentCostConfig
	temperature promm.Gauge
	powerDraw   *promm.GaugeVec
}

func NewCurrentCostCollector(cfg CurrentCostConfig) *CurrentCostCollector {
	monitorLabels := promm.Labels{"monitor": cfg.Name}
	return &CurrentCostCollector{
		cfg: cfg,
		temperature: promm.NewGauge(promm.GaugeOpts{
			Namespace: ccNamespace, Name: "temperature",
			Help:        "Instananeous measured temperature at the monitor (degrees celcius).",
			ConstLabels: monitorLabels,
		}),
		powerDraw: promm.NewGaugeVec(
			promm.GaugeOpts{
				Namespace: ccNamespace, Name: "power_draw",
				Help:        "Instananeous power drawn measured by sensor (watts).",
				ConstLabels: monitorLabels,
			},
			[]string{"sensor", "channel"},
		),
	}
}

func (ccc *CurrentCostCollector) Describe(ch chan<- *promm.Desc) {
	ccc.temperature.Describe(ch)
	ccc.powerDraw.Describe(ch)
}

func (ccc *CurrentCostCollector) Collect(ch chan<- promm.Metric) {
	ccc.temperature.Collect(ch)
	ccc.powerDraw.Collect(ch)
}

func (ccc *CurrentCostCollector) powerDrawReading(sensor, channel int, reading *gocc.Channel) {
	if reading == nil {
		return
	}
	ccc.powerDraw.With(promm.Labels{
		"sensor":  strconv.Itoa(sensor),
		"channel": strconv.Itoa(channel),
	},
	).Set(float64(reading.Watts))
}

// Runs the collector such that it receives updates from the CurrentCost device
// and self-updates. If it returns with an error, it is possible to re-run,
// although some errors might reccur. E.g the device might not exist. This
// could be a permanent or temporary condition.
func (ccc *CurrentCostCollector) Run() error {
	msgReader, err := gocc.NewSerialMessageReader(ccc.cfg.Device)
	if err != nil {
		return err
	}
	defer msgReader.Close()

	for {
		msg, err := msgReader.ReadMessage()
		if err != nil {
			return err
		}

		if msg.Temperature != nil {
			ccc.temperature.Set(float64(*msg.Temperature))
		}

		if msg.Sensor != nil && *msg.Sensor >= 0 && msg.ID != nil {
			ccc.powerDrawReading(*msg.Sensor, 1, msg.Channel1)
			ccc.powerDrawReading(*msg.Sensor, 2, msg.Channel2)
			ccc.powerDrawReading(*msg.Sensor, 3, msg.Channel3)
		}

		// TODO: Consider outputting historical data by accumulating their values
		// into counters.
	}
}
