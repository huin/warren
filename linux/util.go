package linux

import (
	"io/ioutil"
	"log"
	"strconv"
	"strings"

	promm "github.com/prometheus/client_golang/prometheus"
)

func readIntFileIntoCounter(ctr promm.Counter, path string) {
	value, err := readIntFile(path)
	if err != nil {
		log.Printf("Unable to read integer from file %q for counter %v: %v",
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
