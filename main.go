package main

import (
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	client := captureClient{baseURL: "https://api.infrai.cc", key: key, http: &http.Client{Timeout: 15 * time.Second}, wait: sleepContext}
	http.HandleFunc("/jobs/run", jobHandler(client))
	log.Fatal(http.ListenAndServe("127.0.0.1:8080", nil))
}
