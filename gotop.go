package main

import (
	"container/list"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/joliv/spark"
	tb "github.com/nsf/termbox-go"
)

var (
	url          string
	pollInterval time.Duration
)

func init() {
	flag.StringVar(&url, "url", "", "Full url returning expvar JSON")
	flag.DurationVar(&pollInterval, "p", 1*time.Second, "How often to poll")
}

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

func fetchLoop(interval time.Duration, url string, infoChan chan Info) {
	ticker := time.NewTicker(interval)
	for {
		select {
		case <-ticker.C:
			i, err := httpGet(url)
			if err != nil {
				log.Println(err)
				continue
			}
			infoChan <- *i
		}
	}
}

type history struct {
	data *list.List
	size int
	mtx  sync.RWMutex
}

func newHistory(size int) *history {
	return &history{
		data: list.New(),
		size: size,
	}
}

func (h *history) Add(v float64) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	h.data.PushFront(v)
	if h.data.Len() == h.size+1 {
		h.data.Remove(h.data.Back())
	}
}

func (h *history) Spark() string {
	h.mtx.RLock()
	defer h.mtx.RUnlock()
	data := make([]float64, h.data.Len())
	var i int
	for e := h.data.Back(); e != nil; e = e.Prev() {
		data[i] = e.Value.(float64)
		i++
	}
	return spark.Line(data)
}

var (
	heapAllocHistory = newHistory(60)
	stackHistory     = newHistory(60)
)

func draw(info Info) {
	var y int

	for i, r := range fmt.Sprintf("HeapAlloc  : %d", info.MemStats.HeapAlloc) {
		tb.SetCell(i, y, r, tb.ColorDefault, tb.ColorDefault)
	}

	y++
	for i, r := range fmt.Sprintf("StackInUse : %d", info.MemStats.StackInuse) {
		tb.SetCell(i, y, r, tb.ColorDefault, tb.ColorDefault)
	}

	y++
	lastGCTime := time.Unix(0, int64(info.MemStats.LastGC))
	for i, r := range fmt.Sprintf("LastGC     : %v", lastGCTime) {
		tb.SetCell(i, y, r, tb.ColorDefault, tb.ColorDefault)
	}

	y++
	for i, r := range fmt.Sprintf("NextGC     : %d", info.MemStats.NextGC) {
		tb.SetCell(i, y, r, tb.ColorDefault, tb.ColorDefault)
	}
	y++

	y += 4
	// Draw sparklines
	// TODO: Try doubling or tripling the height
	y++
	for i, r := range "HeapAlloc History" {
		tb.SetCell(i, y, r, tb.ColorDefault, tb.ColorDefault)
	}

	y++
	heapAllocHistory.Add(float64(info.MemStats.HeapAlloc))
	// The index given is the byte index, not rune index.
	i := 0
	for _, r := range heapAllocHistory.Spark() {
		tb.SetCell(i, y, r, tb.ColorDefault, tb.ColorDefault)
		i++
	}

	y++
	for i, r := range "Stack History" {
		tb.SetCell(i, y, r, tb.ColorDefault, tb.ColorDefault)
	}

	y++
	stackHistory.Add(float64(info.MemStats.StackInuse))
	i = 0
	for _, r := range stackHistory.Spark() {
		tb.SetCell(i, y, r, tb.ColorDefault, tb.ColorDefault)
		i++
	}
}

func drawLoop(interval time.Duration, infoChan chan Info) {
	ticker := time.NewTicker(interval)
	for {
		select {
		case <-ticker.C:
			err := tb.Flush()
			if err != nil {
				log.Println(err)
			}
		case info := <-infoChan:
			draw(info)
		}
	}
}

func eventLoop() {
	for {
		ev := tb.PollEvent()
		switch ev.Type {
		case tb.EventKey:
			switch ev.Key {
			case tb.KeyEsc, tb.KeyCtrlC:
				tb.Close()
				os.Exit(0)
			}
		}
	}
}

func main() {
	flag.Parse()
	if url == "" {
		log.Fatal("url required")
	}

	tb.Init()
	defer tb.Close()

	for i, r := range "Waiting..." {
		tb.SetCell(i, 0, r, tb.ColorDefault, tb.ColorDefault)
	}
	for i, r := range "Press ESC to quit" {
		tb.SetCell(i, 1, r, tb.ColorDefault, tb.ColorDefault)
	}

	err := tb.Flush()
	if err != nil {
		log.Fatal(err)
	}

	infoChan := make(chan Info)
	go fetchLoop(pollInterval, url, infoChan)
	go eventLoop()
	drawLoop(1*time.Second, infoChan)
}
