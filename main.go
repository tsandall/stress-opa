package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	mathrand "math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/open-policy-agent/opa/metrics"
)

var inputs = generateInputs(10000)

func pickInput() interface{} {
	return inputs[mathrand.Intn(len(inputs))]
}

func generateInputs(n int) []interface{} {

	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}
	paths := make([][]string, 10000)
	users := make([]string, 1000)

	for i := range paths {
		paths[i] = []string{"resources", uuid4()}
	}

	for i := range users {
		users[i] = uuid4()
	}

	result := make([]interface{}, n)

	for i := range result {
		result[i] = map[string]interface{}{
			"input": map[string]interface{}{
				"method": methods[mathrand.Intn(len(methods))],
				"path":   paths[mathrand.Intn(len(paths))],
				"user":   users[mathrand.Intn(len(users))],
			}}
	}

	return result
}

type result struct {
	Total   int64
	Metrics map[string]int64 `json:"metrics,omitempty"`
}

func run(i int, ch chan<- result) {
	client := &http.Client{}
	for {
		func() {
			var buf bytes.Buffer
			input := pickInput()
			if err := json.NewEncoder(&buf).Encode(input); err != nil {
				panic(err)
			}
			t0 := time.Now()
			resp, err := client.Post("http://localhost:8181/v1/data/example/allow?metrics=true", "application/json", &buf)
			if err != nil {
				panic(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				panic(err)
			}
			var r result
			if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
				panic(err)
			}
			r.Total = int64(time.Since(t0))
			ch <- r
		}()
	}
}

func printHeader(keys []string) {
	for i := range keys {
		fmt.Printf("%-14s ", keys[i])
	}
	fmt.Print("\n")
	for i := range keys {
		fmt.Printf("%-14s ", strings.Repeat("-", len(keys[i])))
	}
	fmt.Print("\n")
}

func printRow(keys []string, row map[string]interface{}) {
	for _, k := range keys {
		fmt.Printf("%-14v ", row[k])
	}
	fmt.Print("\n")
}

func main() {

	monitor := make(chan result)

	metricKeys := []string{
		"rps",
		"cli(mean)",
		"cli(90%)",
		"cli(99%)",
		"cli(99.9%)",
		"opa(mean)",
		"opa(90%)",
		"opa(99%)",
		"opa(99.9%)",
	}

	printHeader(metricKeys)

	go func() {
		delay := time.Second * 10
		ticker := time.NewTicker(delay)
		var n int64
		m := metrics.New()
		tLast := time.Now()
		for {
			select {
			case <-ticker.C:

				now := time.Now()
				dt := int64(now.Sub(tLast))
				rps := int64((float64(n) / float64(dt)) * 1e9)

				row := map[string]interface{}{
					"rps": rps,
				}

				hists := []string{"cli", "opa"}

				for _, h := range hists {
					hist := m.Histogram(h).Value().(map[string]interface{})
					keys := []string{"mean", "90%", "99%", "99.9%"}
					for i := range keys {
						row[fmt.Sprintf("%v(%v)", h, keys[i])] = time.Duration(hist[keys[i]].(float64))
					}
				}

				printRow(metricKeys, row)

				tLast = now
				n = 0
				m = metrics.New()

			case r := <-monitor:
				m.Histogram("cli").Update(r.Total)
				ns := int64(0)
				for k, v := range r.Metrics {
					m.Histogram(k).Update(v)
					ns += v
				}
				m.Histogram("opa").Update(ns)
				n++
			}
		}
	}()

	for i := 0; i < 10; i++ {
		go run(i, monitor)
	}

	eof := make(chan struct{})
	<-eof
}

func uuid4() string {
	bs := make([]byte, 16)
	n, err := io.ReadFull(rand.Reader, bs)
	if n != len(bs) || err != nil {
		panic(err)
	}
	bs[8] = bs[8]&^0xc0 | 0x80
	bs[6] = bs[6]&^0xf0 | 0x40
	return fmt.Sprintf("%x-%x-%x-%x-%x", bs[0:4], bs[4:6], bs[6:8], bs[8:10], bs[10:])
}
