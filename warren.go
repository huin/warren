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
	ifl "github.com/influxdb/influxdb-go"
)

var (
	configFile = flag.String("config", "", "Path to configuration file")
)

type Config struct {
	LogPath     string
	InfluxDB    ifl.ClientConfig
	System      *SystemConfig
	CurrentCost []CurrentCostConfig
}

func influxSender(cfg ifl.ClientConfig, influxChan <-chan []*ifl.Series) {
	for {
		influxdb, err := ifl.NewClient(&cfg)
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

func monitorLoop(desc string, fn func() error) {
	for {
		if err := fn(); err != nil {
			log.Printf("%s monitoring error (restarting): %v", desc, err)
		} else {
			log.Printf("%s returned without error (restarting)", desc)
		}
		// Avoid tightlooping on recurring failure.
		time.Sleep(5 * time.Second)
	}
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

	log.Printf("Starting %d Current Cost configurations", len(config.CurrentCost))
	for i := range config.CurrentCost {
		cfgCopy := config.CurrentCost[i]
		go monitorLoop("Current Cost", func() error {
			return currentCost(cfgCopy, influxChan)
		})
	}

	if config.System != nil {
		log.Print("Starting local system monitoring")
		go systemMon(*config.System, influxChan)
	}

	log.Print("Starting InfluxDB sender")
	influxSender(config.InfluxDB, influxChan)
}
