package linux

import (
	"io/ioutil"
	"log"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	promm "github.com/prometheus/client_golang/prometheus"
)

const (
	namespace           = "host"
	netPathSysClassNet  = "/sys/class/net"
	netPathStatsTxBytes = "statistics/tx_bytes"
	netPathStatsRxBytes = "statistics/rx_bytes"
	intFileEnding       = "\x00\n"
)

type Config struct {
	Filesystems []string
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
}

func NewLinuxCollector(cfg Config) *LinuxCollector {
	fsLabelNames := []string{"mount"}
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
	}
}

func (sc *LinuxCollector) Describe(ch chan<- *promm.Desc) {
	sc.fsStatOps.Describe(ch)
	sc.fsSizeBytes.Describe(ch)
	sc.fsFreeBytes.Describe(ch)
	sc.fsUnprivFreeBytes.Describe(ch)
	sc.ifaceTxBytes.Describe(ch)
	sc.ifaceRxBytes.Describe(ch)
}

func (sc *LinuxCollector) Collect(ch chan<- promm.Metric) {
	// Filesystems
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
	// Networks
	if ifaces, err := net.Interfaces(); err != nil {
		log.Print("Error getting network interfaces: %v", err)
	} else {
		for _, iface := range ifaces {
			readIntFileIntoCounter(
				sc.ifaceTxBytes.With(promm.Labels{"interface": iface.Name}),
				filepath.Join(netPathSysClassNet, iface.Name, netPathStatsTxBytes))
			readIntFileIntoCounter(
				sc.ifaceRxBytes.With(promm.Labels{"interface": iface.Name}),
				filepath.Join(netPathSysClassNet, iface.Name, netPathStatsRxBytes))
		}
	}

	sc.fsStatOps.Collect(ch)
	sc.fsSizeBytes.Collect(ch)
	sc.fsFreeBytes.Collect(ch)
	sc.fsUnprivFreeBytes.Collect(ch)
	sc.ifaceTxBytes.Collect(ch)
	sc.ifaceRxBytes.Collect(ch)
}

func readIntFileIntoCounter(ctr promm.Counter, path string) {
	value, err := readIntFile(path)
	if err != nil {
		log.Printf("Unable to read integer from file %q for counter %s: %v",
			path, *ctr.Desc(), err)
		return
	}
	ctr.Set(float64(value))
}

// Read a text file containing a single decimal integer. The number is assumed
// to end at the first of: nul-zero byte, newline, or EOF. The file is read
// into memory so should be short.
func readIntFile(path string) (int64, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return 0, nil
	}
	s := string(data)
	end := strings.IndexAny(s, intFileEnding)
	if end < 0 {
		end = len(s)
	}
	return strconv.ParseInt(s[0:end], 10, 64)
}
