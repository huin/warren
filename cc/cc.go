package cc

import (
	"fmt"
	"strconv"

	"github.com/huin/gocc"
	promm "github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "currentcost"
)

type Config struct {
	Device string
	Labels promm.Labels
	Sensor map[string]Sensor
}

type Sensor struct {
	Name string
}

type CurrentCostCollector struct {
	cfg             Config
	sensorCfgs      map[int]Sensor
	histSensorsSeen map[int]struct{}
	lastSeenDsb     int
	realtimeUpdates *promm.CounterVec
	historyUpdates  promm.Counter
	temperature     promm.Gauge
	powerDraw       *promm.GaugeVec
	powerUsage      *promm.CounterVec
}

func NewCurrentCostCollector(cfg Config) (*CurrentCostCollector, error) {
	sensorCfgs := map[int]Sensor{}
	for sensorIdStr, sensorCfg := range cfg.Sensor {
		sensorId, err := strconv.Atoi(sensorIdStr)
		if err != nil || sensorId < 0 {
			return nil, fmt.Errorf("bad sensor ID %q - must be an integer >= 0")
		}
		sensorCfgs[sensorId] = sensorCfg
	}
	return &CurrentCostCollector{
		cfg:             cfg,
		sensorCfgs:      sensorCfgs,
		histSensorsSeen: map[int]struct{}{},
		lastSeenDsb:     -1,
		realtimeUpdates: promm.NewCounterVec(
			promm.CounterOpts{
				Namespace: namespace, Name: "realtime_by_sensor_count",
				Help:        "Count of realtime updates received, by sensor. (count)",
				ConstLabels: cfg.Labels,
			},
			[]string{"sensor"},
		),
		historyUpdates: promm.NewCounter(
			promm.CounterOpts{
				Namespace: namespace, Name: "history_count",
				Help:        "Count of historical updates received. (count)",
				ConstLabels: cfg.Labels,
			},
		),
		temperature: promm.NewGauge(promm.GaugeOpts{
			Namespace: namespace, Name: "temperature_degc",
			Help:        "Instananeous measured temperature at the monitor. (degrees celcius)",
			ConstLabels: cfg.Labels,
		}),
		powerDraw: promm.NewGaugeVec(
			promm.GaugeOpts{
				Namespace: namespace, Name: "power_draw_watts",
				Help:        "Instananeous power drawn measured by sensor. (watts)",
				ConstLabels: cfg.Labels,
			},
			[]string{"sensor", "channel"},
		),
		powerUsage: promm.NewCounterVec(
			promm.CounterOpts{
				Namespace: namespace, Name: "power_usage_kwhr",
				Help: "Cumulative (sum of all channels) power usage measured by sensor. " +
					"This is accumulated from the latest 2-hourly historical data, so the " +
					"timeseries resolution is coarse. (kilowatt hours)",
				ConstLabels: cfg.Labels,
			},
			[]string{"sensor"},
		),
	}, nil
}

func (ccc *CurrentCostCollector) Describe(ch chan<- *promm.Desc) {
	ccc.realtimeUpdates.Describe(ch)
	ccc.historyUpdates.Describe(ch)
	ccc.temperature.Describe(ch)
	ccc.powerDraw.Describe(ch)
	ccc.powerUsage.Describe(ch)
}

func (ccc *CurrentCostCollector) Collect(ch chan<- promm.Metric) {
	ccc.realtimeUpdates.Collect(ch)
	ccc.historyUpdates.Collect(ch)
	ccc.temperature.Collect(ch)
	ccc.powerDraw.Collect(ch)
	ccc.powerUsage.Collect(ch)
}

func (ccc *CurrentCostCollector) powerDrawReading(sensorName string, channel int, reading *gocc.Channel) {
	if reading == nil {
		return
	}
	ccc.powerDraw.With(promm.Labels{
		"sensor":  sensorName,
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

		// Reset counters if DaysSinceBirth drops between readings.
		if msg.DaysSinceBirth < ccc.lastSeenDsb {
			for sensor := range ccc.histSensorsSeen {
				ccc.powerUsage.With(promm.Labels{"sensor": strconv.Itoa(sensor)}).Set(0)
			}
		}
		ccc.lastSeenDsb = msg.DaysSinceBirth

		if msg.History == nil {
			ccc.processRealtimeData(msg)
		} else {
			ccc.processHistoricalData(msg)
		}
	}
}

func (ccc *CurrentCostCollector) sensorName(sensor int) string {
	sensorCfg, ok := ccc.sensorCfgs[sensor]
	if !ok {
		return strconv.Itoa(sensor)
	}
	return sensorCfg.Name
}

func (ccc *CurrentCostCollector) processRealtimeData(msg *gocc.Message) {
	if msg.Sensor == nil || *msg.Sensor < 0 {
		return
	}

	sensorName := ccc.sensorName(*msg.Sensor)
	ccc.realtimeUpdates.With(promm.Labels{"sensor": sensorName}).Inc()

	if msg.Temperature != nil {
		ccc.temperature.Set(float64(*msg.Temperature))
	}

	ccc.powerDrawReading(sensorName, 1, msg.Channel1)
	ccc.powerDrawReading(sensorName, 2, msg.Channel2)
	ccc.powerDrawReading(sensorName, 3, msg.Channel3)
}

// Produce cumulative power usage by accumulating most recent two-hourly data
// into counters.
func (ccc *CurrentCostCollector) processHistoricalData(msg *gocc.Message) {
	ccc.historyUpdates.Inc()
	for _, sensorHist := range msg.History.Sensors {
		ccc.histSensorsSeen[sensorHist.Sensor] = struct{}{}
		for _, point := range sensorHist.Points {
			u, o, err := point.Time()
			if err != nil {
				continue
			}
			if u == gocc.HistTimeHour && o == 2 {
				// We've found the data we want from this sensor's history (last
				// 2-hours accumulated usage).
				sensorName := ccc.sensorName(sensorHist.Sensor)
				ccc.powerUsage.With(promm.Labels{"sensor": sensorName}).Add(float64(point.Value))
				break
			}
		}
	}
}
