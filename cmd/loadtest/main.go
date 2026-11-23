// loadtest is a local-only HTTP exerciser; it is not included in the production image.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"time"
)

func main() {
	endpoint := flag.String("url", "", "local /source/{id} URL")
	duration := flag.Duration("duration", 5*time.Second, "duration")
	rate := flag.Int("rps", 10000, "offered requests per second")
	workers := flag.Int("workers", 64, "HTTP workers")
	unique := flag.Bool("unique-ua", false, "unique User-Agent per request")
	flag.Parse()
	u, e := url.Parse(*endpoint)
	if e != nil || u.Scheme != "http" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" && u.Hostname() != "::1") {
		log.Fatal("only local HTTP URLs are allowed")
	}
	if *rate < 1 || *workers < 1 || *duration <= 0 {
		log.Fatal("positive rps, workers and duration required")
	}
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{MaxIdleConns: *workers, MaxIdleConnsPerHost: *workers, MaxConnsPerHost: *workers}}
	defer client.CloseIdleConnections()
	jobs := make(chan int, *workers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	latencies := []float64{}
	codes := map[int]int{}
	failed := 0
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := range jobs {
				ua := "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/66.0.3359.139 Safari/537.36"
				if *unique {
					ua += fmt.Sprintf(" local-test/%d", n)
				}
				body, _ := json.Marshal(map[string]any{"id": fmt.Sprintf("load-%d", n), "type": "popunder", "sub": 445566, "ip": "81.2.69.160", "ua": ua})
				start := time.Now()
				res, err := client.Post(*endpoint, "application/json", bytes.NewReader(body))
				status := 0
				if err == nil {
					status = res.StatusCode
					_, err = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
					res.Body.Close()
				}
				elapsed := float64(time.Since(start).Microseconds()) / 1000
				mu.Lock()
				latencies = append(latencies, elapsed)
				codes[status]++
				if err != nil || status != 200 && status != 204 {
					failed++
				}
				mu.Unlock()
			}
		}()
	}
	start := time.Now()
	ticker := time.NewTicker(time.Millisecond)
	scheduled := 0
	dropped := 0
	for time.Since(start) < *duration {
		<-ticker.C
		target := int(time.Since(start).Seconds() * float64(*rate))
		for scheduled < target {
			select {
			case jobs <- scheduled:
			default:
				dropped++
			}
			scheduled++
		}
	}
	ticker.Stop()
	close(jobs)
	wg.Wait()
	elapsed := time.Since(start).Seconds()
	sort.Float64s(latencies)
	percentile := func(p float64) float64 {
		if len(latencies) == 0 {
			return 0
		}
		return latencies[int(float64(len(latencies)-1)*p)]
	}
	out := map[string]any{"unique_ua": *unique, "offered_rps": *rate, "completed": len(latencies), "elapsed_seconds": elapsed, "completed_rps": float64(len(latencies)) / elapsed, "unsent_due_to_saturation": dropped, "failed": failed, "http_statuses": codes, "latency_ms": map[string]float64{"p50": percentile(.5), "p95": percentile(.95), "p99": percentile(.99)}}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(b))
}
