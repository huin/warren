package main

import (
	"log"
	"syscall"
	"time"

	ifl "github.com/influxdb/influxdb-go"
)

type SystemConfig struct {
	Name        string
	Interval    duration
	Filesystems []string
}

func systemMon(cfg SystemConfig, influxChan chan<- []*ifl.Series) {
	interval := cfg.Interval.Duration
	if interval <= 0 {
		oldInterval := interval
		interval = 10 * time.Minute
		log.Print("System monitoring interval %v <= 0, defaulting to %v", oldInterval, interval)
	}

	for _ = range time.Tick(interval) {
		// Ignore the time from the ticker, in case we were blocked doing
		// something else when the tick was generated.
		now := time.Now().Unix()

		var series []*ifl.Series

		if len(cfg.Filesystems) > 0 {
			series = append(series, fsSeries(now, cfg.Name, cfg.Filesystems))
		}

		log.Print(series[0])
		influxChan <- series
	}
}

func fsSeries(now int64, systemName string, filesystems []string) *ifl.Series {
	series := &ifl.Series{
		Name: "system_fs",
		Columns: []string{
			"now", "system", "fs",
			"size_bytes", "free_bytes", "unpriv_free_bytes",
			"files", "files_free",
		},
		Points: make([][]interface{}, 0, len(filesystems)),
	}
	for _, fs := range filesystems {
		var stat syscall.Statfs_t
		if err := syscall.Statfs(fs, &stat); err != nil {
			log.Print("Error stating filesystem %q: %v", fs, err)
			continue
		}
		bs := uint64(stat.Bsize)
		series.Points = append(series.Points, []interface{}{
			now, systemName, fs,
			bs * stat.Blocks, bs * stat.Bfree, stat.Bavail,
			stat.Files, stat.Ffree,
		})
	}
	return series
}
