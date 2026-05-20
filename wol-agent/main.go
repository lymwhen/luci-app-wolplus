package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"
)

var startTime = time.Now()

type StatusResponse struct {
	Online   bool   `json:"online"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Uptime   int64  `json:"uptime"`
}

type ActionResponse struct {
	Success bool   `json:"success"`
	Action  string `json:"action"`
	Delay   int    `json:"delay"`
	Message string `json:"message"`
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	hostname, _ := os.Hostname()
	resp := StatusResponse{
		Online:   true,
		Hostname: hostname,
		OS:       runtime.GOOS,
		Uptime:   int64(time.Since(startTime).Seconds()),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := ActionResponse{
		Success: true,
		Action:  "shutdown",
		Delay:   5,
		Message: fmt.Sprintf("System will shutdown in %d seconds", 5),
	}

	go func() {
		time.Sleep(500 * time.Millisecond)
		exec.Command("shutdown", "/s", "/t", "5").Run()
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func main() {
	port := flag.String("port", "32249", "listen port")
	flag.Parse()

	http.HandleFunc("/api/v1/status", handleStatus)
	http.HandleFunc("/api/v1/shutdown", handleShutdown)

	addr := fmt.Sprintf(":%s", *port)
	log.Printf("wol-agent listening on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
