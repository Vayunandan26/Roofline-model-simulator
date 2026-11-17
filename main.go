/*
Roofline model simulator. You bring in your values and find out if your model
is compute or memory bound.
This does 1) Create a SVG roofline graph
2) Does mathematical calculations presented in the article https://jax-ml.github.io/scaling-book/tpus/
*/

package main 

import (
	"net/http"
	"log"
	"fmt"
	"strings"
)

func main(){
	mux := http.NewServeMux()
	mux.HandleFunc("/api/calculate", calculateHandler)
	mux.HandleFunc("/api/hardware", hardwareHandler)
	mux.HandleFunc("/", homeHandler)
	mux.HandleFunc("/_reload", reloadHandler)
	go watchFrontend()
	fmt.Println("Roofline simulator started at http://localhost/2684")
	log.Fatal(http.ListenAndServe(":2684", mux))
}

func homeHandler(w http.ResponseWriter, r *http.Request){
	switch URL.path{
	case "/":
		http.Redirect(w,r, "/home", http.StatusFound)
	case "/home":
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, filepath.Join("web", "index.html"))
	default:
		http.NotFound(w,r)
	}
}

type HardwareProfile struct{
	Name		string  
	P_compute	float64 
	M_band		float64 
}

var hardwareProfiles = map[string]HardwareProfile{
	"v5e" : {
		Name: "TPU v5e . BF16", P_compute: 1.97e14, M_band: 8.2e11
	},
	"h100" : {
		Name: "NVIDIA H100 . BF16", P_compute: 9.89e14, M_band: 3.35e12
	}
	"v6e" : {
		Name: "TPU v6e . BF16", P_compute: 9.1e14, M_band: 1.6e12
	}
}

func hardwareHandler(w http.ResponseWriter, r *http.Request){
	if r.Method != http.MethodGet{
		http.Error(w, "use Get", http.StatusMethodNotAllowed)
		return 
	}

	name := strings.TrimPrefix(r.URL.Path, "/api/hardware")
	profile, ok := hardwareProfiles[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
}

/* 
more to be developed :
1. Calculate handler function and helper functions inside it 
2. reload handler 
3. frontend 
*/