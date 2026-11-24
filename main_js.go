//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"math"
	"syscall/js"
)

type calculationRequest struct {
	Peak      float64 `json:"peak"`
	Bandwidth float64 `json:"bw"`
	Matmul
}

func main() {
	js.Global().Set("rooflineCalculate", js.FuncOf(calculateFromBrowser))
	select {}
}

func calculateFromBrowser(this js.Value, args []js.Value) any {
	if len(args) != 1 {
		return errorJSON("expected one JSON input object")
	}
	var input calculationRequest
	if err := json.Unmarshal([]byte(args[0].String()), &input); err != nil {
		return errorJSON("invalid calculation values")
	}
	values := []float64{input.Peak, input.Bandwidth, input.B, input.D, input.F, input.A, input.W, input.O}
	for _, value := range values {
		if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return errorJSON("all hardware, shape, and byte-width values must be finite and positive")
		}
	}
	result := CalculateRoofline(input.Peak, input.Bandwidth, input.Matmul)
	derived := []float64{result.FLOPs, result.BytesLoaded, result.BytesWritten, result.BytesMoved, result.Intensity, result.Ridge, result.ComputeTime, result.MemoryTime, result.Throughput, result.LowerBound, result.UpperBound}
	for _, value := range derived {
		if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return errorJSON("calculation overflowed; use smaller input values")
		}
	}
	result.FormulaHTML = renderFormulaRows(input.Peak, input.Bandwidth, input.Matmul, result)
	result.PlotSVG = renderRooflineSVG(input.Peak, input.Bandwidth, result.Intensity, result.Ridge)
	encoded, err := json.Marshal(result)
	if err != nil {
		return errorJSON(fmt.Sprintf("unable to encode calculation: %v", err))
	}
	return string(encoded)
}

func errorJSON(message string) string {
	encoded, _ := json.Marshal(map[string]string{"error": message})
	return string(encoded)
}
