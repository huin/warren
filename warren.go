// warren takes home monitoring data and feeds it into
// [influxdb](http://influxdb.org/).
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/huin/gocc"
	ifl "github.com/influxdb/influxdb-go"
)

var (
	configFile = flag.String("config", "", "Path to configuration file")
)

type Config struct {
	LogPath     string
	InfluxDB    ifl.ClientConfig
	CurrentCost CurrentCostConfig
}

type CurrentCostConfig struct {
	Device string
}

func wattsReading(now int64, sensor, id, channel, watts int) []interface{} {
	return []interface{}{now, "cc", sensor, id, channel, watts}
}

func currentCost(cfg *CurrentCostConfig, influxChan chan<- []*ifl.Series) error {
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

func influxSender(cfg *ifl.ClientConfig, influxChan <-chan []*ifl.Series) {
	for {
		influxdb, err := ifl.NewClient(cfg)
		if err != nil {
			log.Print("Failed to connect to influxdb: ", err)
		}

		for series := range influxChan {
			if err := influxdb.WriteSeriesWithTimePrecision(series, ifl.Second); err != nil {
				log.Print("Failed to send to influxdb: ", err)
				break
			}
		}

		// Avoid tightlooping on recurring failure.
		time.Sleep(5 * time.Second)
	}
}

func initLogging(logpath string) error {
	if logpath == "" {
		// Leave default logging as STDERR.
		return nil
	}

	f, err := os.OpenFile(logpath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, syscall.S_IWUSR|syscall.S_IRUSR)
	if err != nil {
		return fmt.Errorf("failed to open log file: %v", err)
	}
	log.SetOutput(f)
	return nil
}

func readConfig(filename string) (*Config, error) {
	config := new(Config)
	_, err := toml.DecodeFile(filename, &config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func main() {
	flag.Parse()
	if *configFile == "" {
		log.Fatal("--config is required with a filename")
	}
	config, err := readConfig(*configFile)

	if err != nil {
		log.Fatal("Failed to read configuration: ", err)
	}
	initLogging(config.LogPath)

	influxChan := make(chan []*ifl.Series)

	go influxSender(&config.InfluxDB, influxChan)

	for {
		err := currentCost(&config.CurrentCost, influxChan)
		log.Print("Current Cost monitoring error: ", err)

		// Avoid tightlooping on recurring failure.
		time.Sleep(5 * time.Second)
	}
}
