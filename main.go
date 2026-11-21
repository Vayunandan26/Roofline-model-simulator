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
	"html"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	reloadMu	   sync.Mutex
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

// Matmul describes X[B,D] × Y[D,F] → Z[B,F]. Byte widths are supplied
// independently because activation, weight, and output precision can differ.
type Matmul struct {
	B float64 `json:"B"`
	D float64 `json:"D"`
	F float64 `json:"F"`
	A float64 `json:"a"`
	W float64 `json:"w"`
	O float64 `json:"o"`
}

func (m Matmul) FLOPs() float64        { return 2 * m.B * m.D * m.F }
func (m Matmul) BytesLoaded() float64  { return m.A*m.B*m.D + m.W*m.D*m.F }
func (m Matmul) BytesWritten() float64 { return m.O * m.B * m.F }
func (m Matmul) Intensity() float64    { return m.FLOPs() / (m.BytesLoaded() + m.BytesWritten()) }

// RooflineThroughput returns the ideal throughput ceiling at the given
// arithmetic intensity, in FLOP/s.
func RooflineThroughput(peak, bandwidth, intensity float64) float64 {
	return math.Min(peak, bandwidth*intensity)
}

// CriticalBatch returns the smallest batch size that reaches the compute
// ceiling for this matmul and hardware. It reports false when intensity cannot
// reach the ridge, even as batch size grows.
func CriticalBatch(peak, bandwidth float64, m Matmul) (float64, bool) {
	ridge := peak / bandwidth
	hi := 1.0
	for hi <= 1e12 {
		m.B = hi
		if m.Intensity() >= ridge {
			lo := 0.0
			for i := 0; i < 100; i++ {
				mid := (lo + hi) / 2
				m.B = mid
				if m.Intensity() >= ridge {
					hi = mid
				} else {
					lo = mid
				}
			}
			return hi, true
		}
		hi *= 2
	}
	return 0, false
}

// RooflineResult contains the two hardware ceilings and this workload's
// position against them. Times are in seconds and throughput is FLOP/s.
type RooflineResult struct {
	FLOPs           float64 `json:"flops"`
	BytesLoaded     float64 `json:"load"`
	BytesWritten    float64 `json:"write"`
	BytesMoved      float64 `json:"bytes"`
	Intensity       float64 `json:"intensity"`
	Ridge           float64 `json:"ridge"`
	ComputeTime     float64 `json:"tm"`
	MemoryTime      float64 `json:"tmem"`
	Throughput      float64 `json:"rate"`
	LowerBound      float64 `json:"lower"`
	UpperBound      float64 `json:"upper"`
	Regime          string  `json:"regime"`
	CriticalBatch   float64 `json:"critical_batch"`
	CanReachCompute bool    `json:"can_reach_compute"`
	FormulaHTML     string  `json:"formula_html"`
	PlotSVG         string  `json:"plot_svg"`
}

func CalculateRoofline(peak, bandwidth float64, m Matmul) RooflineResult {
	flops := m.FLOPs()
	loaded, written := m.BytesLoaded(), m.BytesWritten()
	bytes := loaded + written
	computeTime, memoryTime := flops/peak, bytes/bandwidth
	lower := math.Max(computeTime, memoryTime)
	regime := "Compute-bound"
	if memoryTime >= computeTime {
		regime = "Memory-bound"
	}
	criticalBatch, canReachCompute := CriticalBatch(peak, bandwidth, m)
	return RooflineResult{
		FLOPs: flops, BytesLoaded: loaded, BytesWritten: written,
		BytesMoved: bytes, Intensity: flops / bytes, Ridge: peak / bandwidth,
		ComputeTime: computeTime, MemoryTime: memoryTime,
		Throughput: RooflineThroughput(peak, bandwidth, flops/bytes), LowerBound: lower,
		UpperBound: computeTime + memoryTime, Regime: regime,
		CriticalBatch: criticalBatch, CanReachCompute: canReachCompute,
	}
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

func displayNumber(value float64) string { return strconv.FormatFloat(value, 'g', 4, 64) }

func axisNumber(value float64) string {
	switch {
	case value >= 1e12:
		return displayNumber(value/1e12) + "T"
	case value >= 1e9:
		return displayNumber(value/1e9) + "B"
	case value >= 1e6:
		return displayNumber(value/1e6) + "M"
	case value >= 1e3:
		return displayNumber(value/1e3) + "k"
	default:
		return displayNumber(value)
	}
}

func displayDuration(seconds float64) string {
	switch {
	case seconds < 1e-6:
		return fmt.Sprintf("%.4g ns", seconds*1e9)
	case seconds < 1e-3:
		return fmt.Sprintf("%.4g µs", seconds*1e6)
	case seconds < 1:
		return fmt.Sprintf("%.4g ms", seconds*1e3)
	default:
		return fmt.Sprintf("%.4g s", seconds)
	}
}

func formulaRow(name, expression, answer string) string {
	return `<div class="formula-row"><div class="formula-name">` + html.EscapeString(name) +
		`</div><div class="formula-eq">` + html.EscapeString(expression) +
		`<span class="formula-answer">= <b>` + html.EscapeString(answer) + `</b></span></div></div>`
}

func renderFormulaRows(peak, bandwidth float64, m Matmul, r RooflineResult) string {
	b, d, f := displayNumber(m.B), displayNumber(m.D), displayNumber(m.F)
	rows := []string{
		formulaRow("Total operations", "2 × "+b+" × "+d+" × "+f, displayNumber(r.FLOPs)+" FLOPs"),
		formulaRow("Bytes loaded", "aBD + wDF = "+displayNumber(m.A)+"×"+b+"×"+d+" + "+displayNumber(m.W)+"×"+d+"×"+f, displayNumber(r.BytesLoaded)+" bytes"),
		formulaRow("Bytes written", "oBF = "+displayNumber(m.O)+"×"+b+"×"+f, displayNumber(r.BytesWritten)+" bytes"),
		formulaRow("Arithmetic intensity", displayNumber(r.FLOPs)+" ÷ ("+displayNumber(r.BytesLoaded)+" + "+displayNumber(r.BytesWritten)+")", displayNumber(r.Intensity)+" FLOPs/byte"),
		formulaRow("Compute time", "T_math = FLOPs ÷ peak = "+displayNumber(r.FLOPs)+" ÷ "+displayNumber(peak), displayDuration(r.ComputeTime)),
		formulaRow("Memory time", "T_comms = bytes ÷ bandwidth = "+displayNumber(r.BytesMoved)+" ÷ "+displayNumber(bandwidth), displayDuration(r.MemoryTime)),
		formulaRow("Lower / upper time bounds", "max(T_math, T_comms) / (T_math + T_comms)", displayDuration(r.LowerBound)+" / "+displayDuration(r.UpperBound)),
		formulaRow("Hardware ridge point", "peak ÷ bandwidth = "+displayNumber(peak)+" ÷ "+displayNumber(bandwidth), displayNumber(r.Ridge)+" FLOPs/byte"),
	}
	if r.CanReachCompute {
		rows = append(rows, formulaRow("Exact critical batch size", "Smallest B where arithmetic intensity reaches the hardware ridge", displayNumber(r.CriticalBatch)))
	} else {
		rows = append(rows, formulaRow("Compute-bound batch size", "Arithmetic intensity does not reach the hardware ridge for these dimensions and byte widths", "Not reached"))
	}
	return strings.Join(rows, "")
}

func renderRooflineSVG(peak, bandwidth, intensity, ridge float64) string {
	const width, height = 800.0, 380.0
	const left, right, top, bottom = 78.0, 25.0, 24.0, 58.0
	plotWidth, plotHeight := width-left-right, height-top-bottom
	xmin, xmax := math.Min(.01, math.Min(intensity/10, ridge/10)), math.Max(10000, math.Max(intensity*10, ridge*10))
	ymin, ymax := math.Min(1e10, math.Min(peak/1000, bandwidth*xmin/10)), math.Max(1e16, peak*10)
	xmin, xmax = math.Pow10(int(math.Floor(math.Log10(xmin)))), math.Pow10(int(math.Ceil(math.Log10(xmax))))
	ymin, ymax = math.Pow10(int(math.Floor(math.Log10(ymin)))), math.Pow10(int(math.Ceil(math.Log10(ymax))))
	lx0, lx1, ly0, ly1 := math.Log10(xmin), math.Log10(xmax), math.Log10(ymin), math.Log10(ymax)
	x := func(value float64) float64 { return left + (math.Log10(value)-lx0)/(lx1-lx0)*plotWidth }
	y := func(value float64) float64 { return top + plotHeight - (math.Log10(value)-ly0)/(ly1-ly0)*plotHeight }
	var svg strings.Builder
	fmt.Fprintf(&svg, `<svg id="chart" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f" role="img" aria-label="Roofline chart">`, width, height)
	fmt.Fprintf(&svg, `<rect width="%.0f" height="%.0f" fill="white"/>`, width, height)
	for exponent := int(math.Log10(xmin)); exponent <= int(math.Log10(xmax)); exponent++ {
		value, px := math.Pow10(exponent), x(math.Pow10(exponent))
		fmt.Fprintf(&svg, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#d4d0c5"/><text x="%.1f" y="%.1f" text-anchor="middle" fill="#222" font-size="10">%s</text>`, px, top, px, top+plotHeight, px, top+plotHeight+20, displayNumber(value))
	}
	for exponent := int(math.Log10(ymin)); exponent <= int(math.Log10(ymax)); exponent++ {
		value, py := math.Pow10(exponent), y(math.Pow10(exponent))
		fmt.Fprintf(&svg, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#d4d0c5"/><text x="%.1f" y="%.1f" text-anchor="end" fill="#222" font-size="10">%s</text>`, left, py, left+plotWidth, py, left-10, py+4, axisNumber(value))
	}
	fmt.Fprintf(&svg, `<path d="M %.1f %.1f L %.1f %.1f L %.1f %.1f" fill="none" stroke="#111111" stroke-width="3"/>`, x(xmin), y(math.Min(peak, bandwidth*xmin)), x(ridge), y(peak), x(xmax), y(peak))
	px, py := x(intensity), y(RooflineThroughput(peak, bandwidth, intensity))
	fmt.Fprintf(&svg, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#2453ff" stroke-dasharray="5 5"/><text x="%.1f" y="%.1f" fill="#2453ff" font-size="10">ridge</text>`, x(ridge), top, x(ridge), top+plotHeight, x(ridge)+5, top+14)
	fmt.Fprintf(&svg, `<circle cx="%.1f" cy="%.1f" r="7" fill="#ff5573" stroke="white" stroke-width="3"/><text x="%.1f" y="%.1f" fill="#ff5573" font-size="11" font-weight="600">Your workload</text>`, px, py, px+11, py-10)
	fmt.Fprintf(&svg, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="none" stroke="#111"/><text x="%.1f" y="%.1f" text-anchor="middle" fill="#111" font-size="12">Arithmetic intensity (FLOPs / byte)</text><text transform="translate(18 %.1f) rotate(-90)" text-anchor="middle" fill="#111" font-size="12">Peak achievable FLOP/s</text></svg>`, left, top, plotWidth, plotHeight, left+plotWidth/2, height-12, top+plotHeight/2)
	return svg.String()
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
