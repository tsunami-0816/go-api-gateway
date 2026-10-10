package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

var requestCount int64

func main() {
	port := "9001"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt64(&requestCount, 1)
		log.Printf("[backend:%s] request #%d: %s %s from %s", port, count, r.Method, r.URL.Path, r.RemoteAddr)

		// Support ?delay=Ms for slow response test
		if delayStr := r.URL.Query().Get("delay"); delayStr != "" {
			if ms, err := strconv.Atoi(delayStr); err == nil {
				time.Sleep(time.Duration(ms) * time.Millisecond)
			}
		}

		// Support ?status=CODE for custom status code
		if statusStr := r.URL.Query().Get("status"); statusStr != "" {
			if code, err := strconv.Atoi(statusStr); err == nil {
				w.WriteHeader(code)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"backend_port": port,
					"request_count": count,
					"status": code,
					"path": r.URL.Path,
				})
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"backend_port":  port,
			"request_count": count,
			"path":           r.URL.Path,
			"method":         r.Method,
			"timestamp":      time.Now().Format(time.RFC3339),
		})
	})

	addr := ":" + port
	log.Printf("[backend:%s] starting on %s", port, addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[backend:%s] error: %v", port, err)
	}
}
