package main

import (
	"time"

	"github.com/huin/gocc"
	ifl "github.com/influxdb/influxdb-go"
)

type CurrentCostConfig struct {
	Device string
}

func wattsReading(now int64, sensor, id, channel, watts int) []interface{} {
	return []interface{}{now, "cc", sensor, id, channel, watts}
}

func currentCost(cfg CurrentCostConfig, influxChan chan<- []*ifl.Series) error {
	msgReader, err := gocc.NewSerialMessageReader(cfg.Device)
	if err != nil {
		return err
	}
	defer msgReader.Close()

	for {
		msg, err := msgReader.ReadMessage()
		if err != nil {
			return err
		}
		now := time.Now().Unix()

		series := []*ifl.Series{
			{
				Name:    "temperature",
				Columns: []string{"time", "source", "value"},
				Points:  [][]interface{}{{now, "cc", msg.Temperature}},
			},
		}

		if msg.Sensor != nil && *msg.Sensor >= 0 && msg.ID != nil {
			pts := [][]interface{}{}

			if msg.Channel1 != nil {
				pts = append(pts, wattsReading(now, *msg.Sensor, *msg.ID, 1, msg.Channel1.Watts))
			}
			if msg.Channel2 != nil {
				pts = append(pts, wattsReading(now, *msg.Sensor, *msg.ID, 2, msg.Channel2.Watts))
			}
			if msg.Channel3 != nil {
				pts = append(pts, wattsReading(now, *msg.Sensor, *msg.ID, 3, msg.Channel3.Watts))
			}

			if len(pts) > 0 {
				series = append(series, &ifl.Series{
					Name:    "watts",
					Columns: []string{"time", "source", "sensor", "id", "channel", "value"},
					Points:  pts,
				})
			}
		}

		influxChan <- series
	}
}
