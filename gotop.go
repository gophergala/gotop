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
	allocHistory     = newHistory(60)
	heapAllocHistory = newHistory(60)
)

func draw(info Info) {
	alloc := fmt.Sprintf("Alloc      : %d", info.MemStats.Alloc)
	for i, r := range alloc {
		tb.SetCell(i, 0, r, tb.ColorDefault, tb.ColorDefault)
	}

	heapAlloc := fmt.Sprintf("HeapAlloc  : %d", info.MemStats.HeapAlloc)
	for i, r := range heapAlloc {
		tb.SetCell(i, 1, r, tb.ColorDefault, tb.ColorDefault)
	}

	stackInUse := fmt.Sprintf("StackInUse : %d", info.MemStats.StackInuse)
	for i, r := range stackInUse {
		tb.SetCell(i, 2, r, tb.ColorDefault, tb.ColorDefault)
	}

	// Draw sparklines
	// TODO: Try doubling or tripling the height

	allocHistoryTitle := "Alloc History"
	for i, r := range allocHistoryTitle {
		tb.SetCell(i, 6, r, tb.ColorDefault, tb.ColorDefault)
	}

	allocHistory.Add(float64(info.MemStats.Alloc))
	allocHistoryString := allocHistory.Spark()
	// The index given is the byte index, not rune index.
	i := 0
	for _, r := range allocHistoryString {
		tb.SetCell(i, 7, r, tb.ColorDefault, tb.ColorDefault)
		i++
	}

	heapAllocHistoryTitle := "HeapAlloc History"
	for i, r := range heapAllocHistoryTitle {
		tb.SetCell(i, 8, r, tb.ColorDefault, tb.ColorDefault)
	}

	heapAllocHistory.Add(float64(info.MemStats.HeapAlloc))
	heapAllocHistoryString := heapAllocHistory.Spark()
	// The index given is the byte index, not rune index.
	i = 0
	for _, r := range heapAllocHistoryString {
		tb.SetCell(i, 9, r, tb.ColorDefault, tb.ColorDefault)
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
			if ev.Key == tb.KeyEsc {
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

	waiting := "Waiting..."
	for i, r := range waiting {
		tb.SetCell(i, 0, r, tb.ColorDefault, tb.ColorDefault)
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
