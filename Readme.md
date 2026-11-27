# Roofline Model Simulator

An interactive **Roofline Model Simulator** built with **Go** and **WebAssembly (WASM)** to visualize hardware compute/memory ceilings, inspect cache-level roofs (L1/L2/L3), and diagnose whether compute workloads are **DRAM memory-bound**, **cache-resident**, or **compute-bound**.

---

## Overview

The [Roofline Model](https://en.wikipedia.org/wiki/Roofline_model) characterizes the attainable performance $P$ of a kernel on a target processor based on its **Peak Compute** ($\pi$, FLOP/s), **Peak Memory Bandwidth** ($\beta$, Bytes/s), and the kernel's **Operational Intensity** ($I = \text{FLOPs} / \text{Bytes}$):

$$P = \min(\pi,\; \beta \times I)$$

The transition between memory-bound and compute-bound execution occurs at the **Ridge Point**:

$$I_{\text{ridge}} = \frac{\pi}{\beta}$$

---

## Features

- **Go Core Engine (`core.go`):** Computes exact log-log roofline curves (including exact ridge-point knees), L1/L2/L3 cache roofs, bottleneck classifications, and actionable optimization recommendations.
- **WebAssembly Execution (`main_js.go`):** Compiles the Go engine to `roofline.wasm` so the browser runs the exact same calculation logic as the native Go CLI, with automatic MIME-safe loading and JS fallback.
- **Interactive Log–Log Canvas UI (`web/`):**
  - **5 FP32 Hardware Presets:** NVIDIA RTX 4090, NVIDIA H100 SXM5, Apple M3 Max, AMD EPYC 9654, and Intel Xeon Platinum 8480+, plus full custom hardware overrides.
  - **6 FP32 Kernel Presets:** Vector Addition, SpMV (CSR), 2D 5-Point Stencil, Conv2D (ResNet), FlashAttention, and SGEMM (4096×4096), plus custom kernel plotting.
  - **Cache-Aware Diagnostics:** Detects DRAM-bound, Compute-bound, Cache-resident (L1/L2/L3), and out-of-bounds peak measurements.
  - **Dynamic Auto-Scaling & Interaction:** Auto-scales log-log axes to fit custom hardware/kernels, supports hover tooltips, click-to-inspect points, and toggles for cache roofs, gridlines, and labels.
- **Native Go CLI (`main.go`) & Unit Tests (`core_test.go`):** Run quick preset benchmarks and verify roofline math from the terminal.

---

## Project Structure

```text
Roofline-model-simulator/
├── core.go          # Roofline math, cache-aware kernel analysis, presets
├── core_test.go     # Unit tests for math, ridge points, and edge cases
├── main.go          # Native Go CLI entrypoint (//go:build !js)
├── main_js.go       # WebAssembly JS bindings (//go:build js && wasm)
├── Makefile         # Build, test, and local server commands
├── go.mod           # Go module definition
└── web/
    ├── index.html   # Simulator UI
    ├── style.css    # Dark-mode responsive layout
    ├── app.js       # WASM loader, state management, HiDPI Canvas renderer
    └── wasm_exec.js # Go WebAssembly runtime helper (copied during build)
```

---

## Getting Started

### Prerequisites

- [Go](https://go.dev/dl/) 1.21+
- Python 3 (or any static HTTP server) to serve the web UI

### Build & Run the Web Simulator

Using `make`:

```bash
git clone https://github.com/Vayunandan26/Roofline-model-simulator.git
cd Roofline-model-simulator
make serve
```

Then open **http://localhost:8080** in your browser.

Or build manually without `make`:

```bash
mkdir -p output/wasm
GOOS=js GOARCH=wasm go build -o output/wasm/roofline.wasm .
cp output/wasm/roofline.wasm web/roofline.wasm

# Copy wasm_exec.js matching your Go version:
# Go 1.24+:
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/wasm_exec.js
# Go 1.23 and earlier:
# cp "$(go env GOROOT)/misc/wasm/wasm_exec.js" web/wasm_exec.js

python3 -m http.server 8080 --directory web
```

### Run the Native Go CLI

```bash
go run .
```

### Run Unit Tests

```bash
go test -v ./...
```

---

## License

MIT