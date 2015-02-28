package linux

import (
	"log"
	"net"
	"path/filepath"
	"syscall"

	promm "github.com/prometheus/client_golang/prometheus"
)

const (
	namespace           = "host"
	netPathSysClassNet  = "/sys/class/net"
	netPathStatsTxBytes = "statistics/tx_bytes"
	netPathStatsRxBytes = "statistics/rx_bytes"
)

type Config struct {
	Filesystems []string
	Cpu         CpuConfig
	Labels      promm.Labels
}

type LinuxCollector struct {
	cfg Config
	// Meta-metrics:
	fsStatOps *promm.CounterVec
	// Filesystem metrics:
	fsSizeBytes       *promm.GaugeVec
	fsFreeBytes       *promm.GaugeVec
	fsUnprivFreeBytes *promm.GaugeVec
	fsFiles           *promm.GaugeVec
	fsFilesFree       *promm.GaugeVec
	// Network metrics:
	ifaceTxBytes *promm.CounterVec
	ifaceRxBytes *promm.CounterVec

	cpuCollector *cpuCollector
}

func NewLinuxCollector(cfg Config) (*LinuxCollector, error) {
	fsLabelNames := []string{"mount"}
	cpuCollector, err := newCpuCollector(cfg.Cpu, cfg.Labels)
	if err != nil {
		return nil, err
	}
	return &LinuxCollector{
		cfg: cfg,
		// Meta-metrics:
		fsStatOps: promm.NewCounterVec(
			promm.CounterOpts{
				Namespace: namespace, Name: "fs_stat_ops",
				Help:        "Statfs calls by mount and result (cumulative calls).",
				ConstLabels: cfg.Labels,
			},
			[]string{"mount", "result"},
		),
		// Filesystem metrics:
		fsSizeBytes: promm.NewGaugeVec(
			promm.GaugeOpts{
				Namespace: namespace, Name: "fs_size_bytes",
				Help:        "Filesystem capacity (bytes).",
				ConstLabels: cfg.Labels,
			},
			fsLabelNames,
		),
		fsFreeBytes: promm.NewGaugeVec(
			promm.GaugeOpts{
				Namespace: namespace, Name: "fs_free_bytes",
				Help:        "Filesystem free space (bytes).",
				ConstLabels: cfg.Labels,
			},
			fsLabelNames,
		),
		fsUnprivFreeBytes: promm.NewGaugeVec(
			promm.GaugeOpts{
				Namespace: namespace, Name: "fs_unpriv_free_bytes",
				Help:        "Filesystem unpriviledged free space (bytes).",
				ConstLabels: cfg.Labels,
			},
			fsLabelNames,
		),
		fsFiles: promm.NewGaugeVec(
			promm.GaugeOpts{
				Namespace: namespace, Name: "fs_files",
				Help:        "File count (files).",
				ConstLabels: cfg.Labels,
			},
			fsLabelNames,
		),
		fsFilesFree: promm.NewGaugeVec(
			promm.GaugeOpts{
				Namespace: namespace, Name: "fs_files_free",
				Help:        "File free count (files).",
				ConstLabels: cfg.Labels,
			},
			fsLabelNames,
		),
		// Network metrics:
		ifaceTxBytes: promm.NewCounterVec(
			promm.CounterOpts{
				Namespace: namespace, Name: "net_tx_bytes",
				Help:        "Count of bytes transmitted by network interface (bytes).",
				ConstLabels: cfg.Labels,
			},
			[]string{"interface"},
		),
		ifaceRxBytes: promm.NewCounterVec(
			promm.CounterOpts{
				Namespace: namespace, Name: "net_rx_bytes",
				Help:        "Count of bytes received by network interface (bytes).",
				ConstLabels: cfg.Labels,
			},
			[]string{"interface"},
		),
		cpuCollector: cpuCollector,
	}, nil
}

func (lc *LinuxCollector) Describe(ch chan<- *promm.Desc) {
	lc.fsStatOps.Describe(ch)
	lc.fsSizeBytes.Describe(ch)
	lc.fsFreeBytes.Describe(ch)
	lc.fsUnprivFreeBytes.Describe(ch)
	lc.ifaceTxBytes.Describe(ch)
	lc.ifaceRxBytes.Describe(ch)
	lc.cpuCollector.Describe(ch)
}

func (lc *LinuxCollector) Collect(ch chan<- promm.Metric) {
	// Filesystems
	for _, fs := range lc.cfg.Filesystems {
		var stat syscall.Statfs_t
		if err := syscall.Statfs(fs, &stat); err != nil {
			log.Print("Error stating filesystem %q: %v", fs, err)
			lc.fsStatOps.With(promm.Labels{"mount": fs, "result": "error"}).Inc()
			continue
		}
		lc.fsStatOps.With(promm.Labels{"mount": fs, "result": "ok"}).Inc()
		mountLabels := promm.Labels{"mount": fs}
		bs := uint64(stat.Bsize)
		lc.fsSizeBytes.With(mountLabels).Set(float64(bs * stat.Blocks))
		lc.fsFreeBytes.With(mountLabels).Set(float64(bs * stat.Bfree))
		lc.fsUnprivFreeBytes.With(mountLabels).Set(float64(bs * stat.Bavail))
		lc.fsFiles.With(mountLabels).Set(float64(stat.Files))
		lc.fsFilesFree.With(mountLabels).Set(float64(stat.Ffree))
	}
	// Networks
	if ifaces, err := net.Interfaces(); err != nil {
		log.Print("Error getting network interfaces: %v", err)
	} else {
		for _, iface := range ifaces {
			readIntFileIntoCounter(
				lc.ifaceTxBytes.With(promm.Labels{"interface": iface.Name}),
				filepath.Join(netPathSysClassNet, iface.Name, netPathStatsTxBytes))
			readIntFileIntoCounter(
				lc.ifaceRxBytes.With(promm.Labels{"interface": iface.Name}),
				filepath.Join(netPathSysClassNet, iface.Name, netPathStatsRxBytes))
		}
	}

	lc.fsStatOps.Collect(ch)
	lc.fsSizeBytes.Collect(ch)
	lc.fsFreeBytes.Collect(ch)
	lc.fsUnprivFreeBytes.Collect(ch)
	lc.ifaceTxBytes.Collect(ch)
	lc.ifaceRxBytes.Collect(ch)
	lc.cpuCollector.Collect(ch)
}
