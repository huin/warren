// warren takes home monitoring data and feeds it into
// [seriesly](https://github.com/dustin/seriesly).
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"syscall"
	"time"

	"github.com/huin/gocc"
)

var (
	serieslyUrl  = flag.String("seriesly-url", "", "HTTP URL to Seriesly server database, e.g http://localhost:3133/db")
	ccSerialPort = flag.String("cc-port", "/dev/ttyUSB0", "Filesystem path to Current Cost serial port")
	logPath      = flag.String("log-path", "", "Log to file, default logs to STDERR.")
)

func initLogging() {
	if *logPath == "" {
		// Leave default logging as STDERR.
		return
	}

	f, err := os.OpenFile(*logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, syscall.S_IWUSR|syscall.S_IRUSR)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	log.SetOutput(f)
}

type ccMsg struct {
	Temperature *float32       `json:"temperature,omitempty"`
	Watts       map[string]int `json:"watts,omitempty"`
}

func currentCost() error {
	msgReader, err := gocc.NewSerialMessageReader(*ccSerialPort)
	if err != nil {
		return err
	}
	defer msgReader.Close()

	for {
		msg, err := msgReader.ReadMessage()
		if err != nil {
			return err
		}

		data := &ccMsg{}
		data.Temperature = msg.Temperature
		if msg.Sensor != nil && *msg.Sensor >= 0 && msg.Channel1 != nil {
			data.Watts = map[string]int{strconv.Itoa(*msg.Sensor): msg.Channel1.Watts}
		}

		jsonData, err := json.Marshal(data)
		if len(jsonData) == 2 {
			// No data - "{}".
			continue
		}
		if err != nil {
			log.Printf("Error encoding Current Cost data as JSON: %v", err)
		}
		log.Printf("jsonData = %s", jsonData)
		reqBuf := bytes.NewBuffer(jsonData)
		resp, err := http.Post(*serieslyUrl, "application/json", reqBuf)
		if err != nil {
			log.Printf("Error sending JSON to %s: %v", *serieslyUrl, err)
		} else if resp.StatusCode != http.StatusCreated {
			respBuf := &bytes.Buffer{}
			_, _ = io.CopyN(respBuf, resp.Body, 80)
			log.Printf("HTTP response status %d, body: %s", resp.StatusCode, respBuf.Bytes())
		}
		_, _ = io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}
	panic("unreachable")
}

func main() {
	flag.Parse()
	initLogging()

	if *serieslyUrl == "" {
		log.Fatal("Bad value for --seriesly-url")
	}

	for {
		log.Print("Current Cost monitoring error: %v", currentCost())

		// Avoid tightlooping on recurring failure.
		time.Sleep(5 * time.Second)
	}
}
