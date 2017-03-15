package cc

import (
	"fmt"
	"strconv"

	"github.com/huin/gocc"
	"github.com/huin/warren/util"
	promm "github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "currentcost"
)

type Config struct {
	Device string
	Labels promm.Labels
	Sensor map[string]SensorConfig
}

type SensorConfig struct {
	Name string
}

type Collector struct {
	cfg             Config
	sensorCfgs      map[int]SensorConfig
	histSensorsSeen map[int]struct{}
	lastSeenDsb     int
	metrics         util.MetricCollection
	realtimeUpdates *promm.CounterVec
	historyUpdates  promm.Counter
	temperature     promm.Gauge
	powerDraw       *promm.GaugeVec
	powerUsage      *promm.GaugeVec
}

func New(cfg Config) (*Collector, error) {
	sensorCfgs := map[int]SensorConfig{}
	for sensorIdStr, sensorCfg := range cfg.Sensor {
		sensorId, err := strconv.Atoi(sensorIdStr)
		if err != nil || sensorId < 0 {
			return nil, fmt.Errorf("bad sensor ID %q - must be an integer >= 0", sensorId)
		}
		sensorCfgs[sensorId] = sensorCfg
	}
	var metrics util.MetricCollection
	c := &Collector{
		cfg:             cfg,
		sensorCfgs:      sensorCfgs,
		histSensorsSeen: map[int]struct{}{},
		lastSeenDsb:     -1,
		realtimeUpdates: metrics.NewCounterVec(
			promm.CounterOpts{
				Namespace: namespace, Name: "realtime_by_sensor_count",
				Help:        "Count of realtime updates received, by sensor. (count)",
				ConstLabels: cfg.Labels,
			},
			[]string{"sensor"},
		),
		historyUpdates: metrics.NewCounter(
			promm.CounterOpts{
				Namespace: namespace, Name: "history_count",
				Help:        "Count of historical updates received. (count)",
				ConstLabels: cfg.Labels,
			},
		),
		temperature: metrics.NewGauge(promm.GaugeOpts{
			Namespace: namespace, Name: "temperature_degc",
			Help:        "Instananeous measured temperature at the monitor. (degrees celcius)",
			ConstLabels: cfg.Labels,
		}),
		powerDraw: metrics.NewGaugeVec(
			promm.GaugeOpts{
				Namespace: namespace, Name: "power_draw_watts",
				Help:        "Instananeous power drawn measured by sensor. (watts)",
				ConstLabels: cfg.Labels,
			},
			[]string{"sensor", "channel"},
		),
		powerUsage: metrics.NewGaugeVec(
			promm.GaugeOpts{
				Namespace: namespace, Name: "power_usage_kwhr",
				Help: "Cumulative (sum of all channels) power usage measured by sensor. " +
					"This is accumulated from the latest 2-hourly historical data, so the " +
					"timeseries resolution is coarse. (kilowatt hours)",
				ConstLabels: cfg.Labels,
			},
			[]string{"sensor"},
		),
	}
	c.metrics = metrics
	return c, nil
}

func (c *Collector) Describe(ch chan<- *promm.Desc) {
	c.metrics.Describe(ch)
}

func (c *Collector) Collect(ch chan<- promm.Metric) {
	c.metrics.Collect(ch)
}

func (c *Collector) powerDrawReading(sensorName string, channel int, reading *gocc.Channel) {
	if reading == nil {
		return
	}
	c.powerDraw.With(promm.Labels{
		"sensor":  sensorName,
		"channel": strconv.Itoa(channel),
	},
	).Set(float64(reading.Watts))
}

// Runs the collector such that it receives updates from the CurrentCost device
// and self-updates. If it returns with an error, it is possible to re-run,
// although some errors might reccur. E.g the device might not exist. This
// could be a permanent or temporary condition.
func (c *Collector) Run() error {
	msgReader, err := gocc.NewSerialMessageReader(c.cfg.Device)
	if err != nil {
		return err
	}
	defer msgReader.Close()

	for {
		msg, err := msgReader.ReadMessage()
		if err != nil {
			return err
		}

		// Reset counters if DaysSinceBirth drops between readings.
		if msg.DaysSinceBirth < c.lastSeenDsb {
			for sensor := range c.histSensorsSeen {
				c.powerUsage.With(promm.Labels{"sensor": strconv.Itoa(sensor)}).Set(0)
			}
		}
		c.lastSeenDsb = msg.DaysSinceBirth

		if msg.History == nil {
			c.processRealtimeData(msg)
		} else {
			c.processHistoricalData(msg)
		}
	}
}

func (c *Collector) sensorName(sensor int) string {
	sensorCfg, ok := c.sensorCfgs[sensor]
	if !ok {
		return strconv.Itoa(sensor)
	}
	return sensorCfg.Name
}

func (c *Collector) processRealtimeData(msg *gocc.Message) {
	if msg.Sensor == nil || *msg.Sensor < 0 {
		return
	}

	sensorName := c.sensorName(*msg.Sensor)
	c.realtimeUpdates.With(promm.Labels{"sensor": sensorName}).Inc()

	if msg.Temperature != nil {
		c.temperature.Set(float64(*msg.Temperature))
	}

	c.powerDrawReading(sensorName, 1, msg.Channel1)
	c.powerDrawReading(sensorName, 2, msg.Channel2)
	c.powerDrawReading(sensorName, 3, msg.Channel3)
}

// Produce cumulative power usage by accumulating most recent two-hourly data
// into counters.
func (c *Collector) processHistoricalData(msg *gocc.Message) {
	c.historyUpdates.Inc()
	for _, sensorHist := range msg.History.Sensors {
		c.histSensorsSeen[sensorHist.Sensor] = struct{}{}
		for _, point := range sensorHist.Points {
			u, o, err := point.Time()
			if err != nil {
				continue
			}
			if u == gocc.HistTimeHour && o == 2 {
				// We've found the data we want from this sensor's history (last
				// 2-hours accumulated usage).
				sensorName := c.sensorName(sensorHist.Sensor)
				c.powerUsage.With(promm.Labels{"sensor": sensorName}).Add(float64(point.Value))
				break
			}
		}
	}
}
