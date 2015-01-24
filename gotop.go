package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"runtime"
	"time"
)

type Info struct {
	MemStats runtime.MemStats
}

func parseJSON(r io.Reader) (*Info, error) {
	info := new(Info)
	decoder := json.NewDecoder(r)
	err := decoder.Decode(info)
	if err != nil {
		return nil, fmt.Errorf("decoding JSON: %v", err)
	}
	return info, nil
}

func httpGet(url string) (*Info, error) {
	rsp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("getting JSON: %v", err)
	}
	defer rsp.Body.Close()
	return parseJSON(rsp.Body)
}

func fetchLoop(interval time.Duration, url string) {
	ticker := time.NewTicker(interval)
	for {
		select {
		case <-ticker.C:
			info, err := httpGet(url)
			if err != nil {
				log.Println(err)
				continue
			}
			fmt.Println(info.MemStats.Alloc)
		}
	}
}

func main() {
	fetchLoop(1*time.Second, "http://golang.org/debug/vars")
}
