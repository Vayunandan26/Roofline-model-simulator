//go:build !js || !wasm

/*
Roofline model simulator. You bring in your values and find out if your model
is compute or memory bound.
This does 1) Create a SVG roofline graph
2) Does mathematical calculations presented in the article https://jax-ml.github.io/scaling-book/tpus/
*/

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	reloadMu      sync.Mutex
	reloadClients = map[chan struct{}]struct{}{}
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/calculate", calculateHandler)
	mux.HandleFunc("/api/hardware/", hardwareHandler)
	mux.HandleFunc("/", homeHandler)
	mux.HandleFunc("/__reload", reloadHandler)
	go watchFrontend()
	fmt.Println("Roofline Studio running at http://localhost:2684")
	log.Fatal(http.ListenAndServe(":2684", mux))
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/":
		http.Redirect(w, r, "/home", http.StatusFound)
	case "/home":
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, filepath.Join("web", "index.html"))
	case "/styles.css", "/app.js":
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filepath.Join("web", filepath.Base(r.URL.Path)))
	default:
		http.NotFound(w, r)
	}
}

type HardwareProfile struct {
	Name      string  `json:"name"`
	Peak      float64 `json:"peak"`
	Bandwidth float64 `json:"bw"`
}

var hardwareProfiles = map[string]HardwareProfile{
	"v5e":  {Name: "TPU v5e · BF16", Peak: 1.97e14, Bandwidth: 8.2e11},
	"h100": {Name: "NVIDIA H100 · BF16", Peak: 9.89e14, Bandwidth: 3.35e12},
	"v6e":  {Name: "TPU v6e · BF16", Peak: 9.1e14, Bandwidth: 1.6e12},
}

func hardwareHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "use GET", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/hardware/")
	profile, ok := hardwareProfiles[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(profile)
}

type calculationRequest struct {
	Peak      float64 `json:"peak"`
	Bandwidth float64 `json:"bw"`
	Matmul
}

func calculateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "use POST", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var input calculationRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid calculation values", http.StatusBadRequest)
		return
	}
	values := []float64{input.Peak, input.Bandwidth, input.B, input.D, input.F, input.A, input.W, input.O}
	for _, value := range values {
		if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			http.Error(w, "all hardware, shape, and byte-width values must be finite and positive", http.StatusBadRequest)
			return
		}
	}
	result := CalculateRoofline(input.Peak, input.Bandwidth, input.Matmul)
	derived := []float64{result.FLOPs, result.BytesLoaded, result.BytesWritten, result.BytesMoved, result.Intensity, result.Ridge, result.ComputeTime, result.MemoryTime, result.Throughput, result.LowerBound, result.UpperBound}
	for _, value := range derived {
		if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			http.Error(w, "calculation overflowed; use smaller input values", http.StatusBadRequest)
			return
		}
	}
	result.FormulaHTML = renderFormulaRows(input.Peak, input.Bandwidth, input.Matmul, result)
	result.PlotSVG = renderRooflineSVG(input.Peak, input.Bandwidth, result.Intensity, result.Ridge)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func reloadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is unavailable", http.StatusInternalServerError)
		return
	}
	updates := make(chan struct{}, 1)
	reloadMu.Lock()
	reloadClients[updates] = struct{}{}
	reloadMu.Unlock()
	defer func() {
		reloadMu.Lock()
		delete(reloadClients, updates)
		reloadMu.Unlock()
	}()
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-updates:
			fmt.Fprint(w, "event: reload\ndata: changed\n\n")
			flusher.Flush()
		}
	}
}

func watchFrontend() {
	path := filepath.Join("web", "index.html")
	info, err := os.Stat(path)
	if err != nil {
		log.Printf("watching frontend: %v", err)
		return
	}
	lastModified, lastSize := info.ModTime(), info.Size()
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if !info.ModTime().Equal(lastModified) || info.Size() != lastSize {
			lastModified, lastSize = info.ModTime(), info.Size()
			reloadMu.Lock()
			for client := range reloadClients {
				select {
				case client <- struct{}{}:
				default:
				}
			}
			reloadMu.Unlock()
		}
	}
}
