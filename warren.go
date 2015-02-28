// Warren is a program to act as part of a monitoring system on a home network.
// It exports data for external programs to acquire and log to timeseries
// databases. Currently, Warren exports data in a way that is intended for
// scraping by Prometheus - http://prometheus.io/.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/huin/warren/cc"
	"github.com/huin/warren/linux"
	promm "github.com/prometheus/client_golang/prometheus"
)

var (
	configFile = flag.String("config", "", "Path to configuration file")
)

type Config struct {
	LogPath     string
	Prometheus  PrometheusConfig
	System      *linux.Config
	CurrentCost []cc.Config
}

type PrometheusConfig struct {
	HandlerPath string
	ServeAddr   string
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

	log.Printf("Starting %d CurrentCost collectors", len(config.CurrentCost))
	for _, ccConfig := range config.CurrentCost {
		ccc, err := cc.NewCurrentCostCollector(ccConfig)
		if err != nil {
			log.Fatal("Error in CurrentCost config: %v", err)
		}
		promm.MustRegister(ccc)
		go monitorLoop("Current Cost", func() error {
			return ccc.Run()
		})
	}

	if config.System != nil {
		log.Print("Starting local system monitoring")
		promm.MustRegister(linux.NewLinuxCollector(*config.System))
	}

	log.Print("Starting Prometheus metrics handler")
	http.Handle(config.Prometheus.HandlerPath, promm.Handler())
	http.ListenAndServe(config.Prometheus.ServeAddr, nil)
}
