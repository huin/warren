package main

import (
	"log"
	"syscall"

	promm "github.com/prometheus/client_golang/prometheus"
)

const (
	systemNamespace = "system"
)

type SystemConfig struct {
	Filesystems []string
	Labels      promm.Labels
}

type SystemCollector struct {
	cfg               SystemConfig
	fsStatOps         *promm.CounterVec
	fsSizeBytes       *promm.GaugeVec
	fsFreeBytes       *promm.GaugeVec
	fsUnprivFreeBytes *promm.GaugeVec
	fsFiles           *promm.GaugeVec
	fsFilesFree       *promm.GaugeVec
}

func NewSystemCollector(cfg SystemConfig) *SystemCollector {
	fsLabelNames := []string{"mount"}
	return &SystemCollector{
		cfg: cfg,
		// Meta-metrics:
		fsStatOps: promm.NewCounterVec(
			promm.CounterOpts{
				Namespace: systemNamespace, Name: "fs_stat_ops",
				Help:        "Statfs calls by mount and result (cumulative calls).",
				ConstLabels: cfg.Labels,
			},
			[]string{"mount", "result"},
		),
		// Filesystem metrics:
		fsSizeBytes: promm.NewGaugeVec(
			promm.GaugeOpts{
				Namespace: systemNamespace, Name: "fs_size_bytes",
				Help:        "Filesystem capacity (bytes).",
				ConstLabels: cfg.Labels,
			},
			fsLabelNames,
		),
		fsFreeBytes: promm.NewGaugeVec(
			promm.GaugeOpts{
				Namespace: systemNamespace, Name: "fs_free_bytes",
				Help:        "Filesystem free space (bytes).",
				ConstLabels: cfg.Labels,
			},
			fsLabelNames,
		),
		fsUnprivFreeBytes: promm.NewGaugeVec(
			promm.GaugeOpts{
				Namespace: systemNamespace, Name: "fs_unpriv_free_bytes",
				Help:        "Filesystem unpriviledged free space (bytes).",
				ConstLabels: cfg.Labels,
			},
			fsLabelNames,
		),
		fsFiles: promm.NewGaugeVec(
			promm.GaugeOpts{
				Namespace: systemNamespace, Name: "fs_files",
				Help:        "File count (files).",
				ConstLabels: cfg.Labels,
			},
			fsLabelNames,
		),
		fsFilesFree: promm.NewGaugeVec(
			promm.GaugeOpts{
				Namespace: systemNamespace, Name: "fs_files_free",
				Help:        "File free count (files).",
				ConstLabels: cfg.Labels,
			},
			fsLabelNames,
		),
	}
}

func (sc *SystemCollector) Describe(ch chan<- *promm.Desc) {
	sc.fsSizeBytes.Describe(ch)
	sc.fsFreeBytes.Describe(ch)
	sc.fsUnprivFreeBytes.Describe(ch)
}

func (sc *SystemCollector) Collect(ch chan<- promm.Metric) {
	for _, fs := range sc.cfg.Filesystems {
		var stat syscall.Statfs_t
		if err := syscall.Statfs(fs, &stat); err != nil {
			log.Print("Error stating filesystem %q: %v", fs, err)
			sc.fsStatOps.With(promm.Labels{"mount": fs, "result": "error"}).Inc()
			continue
		}
		sc.fsStatOps.With(promm.Labels{"mount": fs, "result": "ok"}).Inc()
		mountLabels := promm.Labels{"mount": fs}
		bs := uint64(stat.Bsize)
		sc.fsSizeBytes.With(mountLabels).Set(float64(bs * stat.Blocks))
		sc.fsFreeBytes.With(mountLabels).Set(float64(bs * stat.Bfree))
		sc.fsUnprivFreeBytes.With(mountLabels).Set(float64(bs * stat.Bavail))
		sc.fsFiles.With(mountLabels).Set(float64(stat.Files))
		sc.fsFilesFree.With(mountLabels).Set(float64(stat.Ffree))
	}
	sc.fsSizeBytes.Collect(ch)
	sc.fsFreeBytes.Collect(ch)
	sc.fsUnprivFreeBytes.Collect(ch)
}
