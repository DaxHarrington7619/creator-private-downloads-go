package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	delivery "example.com/creator-private-downloads"
)

const bucketName = "creator-private-releases"

func main() {
	client := &delivery.Client{APIKey: os.Getenv("INFRAI_API_KEY"), MaxRetries: 3}
	setupContext, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := client.CreateBucket(setupContext, bucketName); err != nil {
		log.Fatal(err)
	}
	issuer := delivery.DownloadIssuer{Bucket: bucketName, Signer: client, Now: time.Now}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /download-links", func(response http.ResponseWriter, request *http.Request) {
		var input delivery.DownloadRequest
		decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 32<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeJSON(response, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		link, err := issuer.Issue(request.Context(), input)
		if err != nil {
			status := http.StatusBadGateway
			if errors.Is(err, delivery.ErrDownloadDenied) {
				status = http.StatusForbidden
			} else {
				var apiErr *delivery.InfraiError
				if errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500 {
					status = apiErr.Status
				}
			}
			writeJSON(response, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(response, http.StatusOK, link)
	})

	log.Printf("private download service listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	if err := json.NewEncoder(response).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}
