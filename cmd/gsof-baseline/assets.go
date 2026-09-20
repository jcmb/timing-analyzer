package main

import (
	"net/http"
	"strconv"

	"timing-analyzer/web"
)

func serveEmbeddedJS(w http.ResponseWriter, r *http.Request, data []byte) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		return
	}
	_, _ = w.Write(data)
}

func serveBaselineChartJS(w http.ResponseWriter, r *http.Request) {
	serveEmbeddedJS(w, r, web.ChartJS)
}

func serveBaselineHammerJS(w http.ResponseWriter, r *http.Request) {
	serveEmbeddedJS(w, r, web.HammerJS)
}

func serveBaselineChartZoomJS(w http.ResponseWriter, r *http.Request) {
	serveEmbeddedJS(w, r, web.ChartJSPluginZoom)
}
