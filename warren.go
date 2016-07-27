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
	"github.com/huin/warren/httpexport"
	"github.com/huin/warren/linux"
	"github.com/huin/warren/streammatch"
	promm "github.com/prometheus/client_golang/prometheus"
)

var (
	configFile = flag.String("config", "", "Path to configuration file")
)

type Config struct {
	LogPath     string
	Prometheus  PrometheusConfig
	CurrentCost []cc.Config
	File        []streammatch.FileCfg
	Proc        []streammatch.ProcCfg
	System      *linux.Config
	HTTPExport  []httpexport.Config
}

type PrometheusConfig struct {
	HandlerPath string
	// TODO: Deprecate ServeAddr and move into Config - it's not really specific
	// to the Prometheus part of things.
	ServeAddr string
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

func monitorLoop(name string, fn func() error) {
	for {
		if err := fn(); err != nil {
			log.Printf("%s monitoring error (restarting): %v", name, err)
		} else {
			log.Printf("%s returned without error (restarting)", name)
		}
		restartCounter.With(promm.Labels{"name": name}).Inc()
		// Avoid tightlooping on recurring failure.
		time.Sleep(5 * time.Second)
	}
}

var restartCounter *promm.CounterVec

func init() {
	restartCounter = promm.NewCounterVec(
		promm.CounterOpts{
			Namespace: "warren", Name: "running_monitor_restarts_total",
			Help: "Number of times a running monitor has restarted. (count)",
		},
		[]string{"name"},
	)
	promm.MustRegister(restartCounter)
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

	if len(config.CurrentCost) > 0 {
		log.Printf("Starting %d CurrentCost collectors", len(config.CurrentCost))
	}
	for i, cfg := range config.CurrentCost {
		c, err := cc.New(cfg)
		if err != nil {
			log.Fatalf("Error in CurrentCost[%d]: %v", i, err)
		}
		promm.MustRegister(c)
		go monitorLoop("currentcost", c.Run)
	}

	if len(config.File) > 0 {
		log.Printf("Starting %d File collectors", len(config.File))
	}
	for i, cfg := range config.File {
		fc, err := streammatch.NewFileCollector(cfg)
		if err != nil {
			log.Fatalf("Error in File[%d]: %v", i, err)
		}
		promm.MustRegister(fc)
	}

	if len(config.Proc) > 0 {
		log.Printf("Starting %d Proc collectors", len(config.Proc))
	}
	for i, cfg := range config.Proc {
		c, err := streammatch.NewProcCollector(cfg)
		if err != nil {
			log.Fatalf("Error in Proc[%d]: %v", i, err)
		}
		promm.MustRegister(c)
	}

	if config.System != nil {
		log.Print("Starting local system monitoring")
		c, err := linux.New(*config.System)
		if err != nil {
			log.Fatalf("Error in System: %v", err)
		}
		promm.MustRegister(c)
	}

	if len(config.HTTPExport) > 0 {
		log.Printf("Starting %d HTTPExport collectors", len(config.HTTPExport))
	}
	for i, hec := range config.HTTPExport {
		c, err := httpexport.New(hec)
		if err != nil {
			log.Fatalf("Error in HTTPExport[%d]: %v", i, err)
		}
		promm.MustRegister(c)
	}

	log.Print("Starting Prometheus metrics handler")
	http.Handle(config.Prometheus.HandlerPath, promm.Handler())
	http.ListenAndServe(config.Prometheus.ServeAddr, nil)
}
