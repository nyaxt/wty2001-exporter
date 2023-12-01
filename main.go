package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"
)

var apiURI string
var mockResponseFile string

func init() {
	flag.StringVar(&mockResponseFile, "mock", "", "The file to read mock response from")
	flag.StringVar(&apiURI, "target", "http://127.0.0.1:12380/cgi-bin/index.cgi?p=dataget", "The WTY2001 HTTP API endpoint")
}

// Below is the text that is returned by the WTY2001 API.
// javascript:parent.lightValueSet([index], [info_unused], [dimmer_available], [brightness], [name_unused], [via_repeater_unused], '[model_number]+[num_unused].png');
// javascript:parent.lightValueSet(0,1,1,38,'照明1',0,'WTY22473+20.png');
// javascript:parent.lightValueSet(1,1,1,40,'照明2',0,'WTY22473+20.png');
// javascript:parent.lightValueSet(2,1,0,0,'照明3',0,'WTY2201+04.png');
var reParseLine = regexp.MustCompile(`javascript:parent.lightValueSet\((?P<index>\d+),\d+,\d+,(?P<brightness>\d+),'[^']*',\d+,'(?P<model_number>[^\+]+)\+\d+\.png'\);`)

type LightStatus struct {
	Index       int
	Brightness  int
	ModelNumber string
}

func ParseAPIResponse(bs []byte) ([]LightStatus, error) {
	ret := []LightStatus{}

	scanner := bufio.NewScanner(bytes.NewReader(bs))
	for scanner.Scan() {
		line := scanner.Text()

		match := reParseLine.FindStringSubmatch(line)
		if len(match) == 0 {
			continue
		}

		indexStr, brightnessStr, modelNumberStr := match[1], match[2], match[3]
		index, err := strconv.Atoi(indexStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse index %s: %w", indexStr, err)
		}
		brightness, err := strconv.Atoi(brightnessStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse brightness %s: %w", brightnessStr, err)
		}

		ret = append(ret, LightStatus{
			Index:       index,
			Brightness:  brightness,
			ModelNumber: modelNumberStr,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return ret, nil
}

func CallAPI() ([]LightStatus, error) {
	var bs []byte
	if mockResponseFile != "" {
		var err error
		bs, err = ioutil.ReadFile(mockResponseFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read mock response file %s: %w", mockResponseFile, err)
		}
	} else {
		resp, err := http.Get(apiURI)
		if err != nil {
			return nil, fmt.Errorf("failed to get %s: %w", apiURI, err)
		}
		bs, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}
		resp.Body.Close()
	}

	return ParseAPIResponse(bs)
}

func HandleMetrics(w http.ResponseWriter, r *http.Request) {
	ls, err := CallAPI()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "# TYPE light_brightness gauge\n")
	for _, l := range ls {
		fmt.Fprintf(w, "light_brightness{index=\"%d\",model_number=\"%s\"} %d\n", l.Index, l.ModelNumber, l.Brightness)
	}
}

func main() {
	flag.Parse()

	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "ok")
	})
	http.HandleFunc("/metrics", HandleMetrics)

	http.ListenAndServe(":8080", nil)
}
