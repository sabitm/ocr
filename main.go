package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func init() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: ocr <image-path>\n")
		os.Exit(1)
	}
	imagePath := os.Args[1]

	client := &http.Client{Timeout: 120 * time.Second}
	baseURL := pickGradioURL()

	serverPath, err := upload(client, baseURL, imagePath)
	if err != nil {
		log.Fatalf("upload failed: %v", err)
	}

	result, err := queuePredict(client, baseURL, serverPath, imagePath)
	if err != nil {
		log.Fatalf("predict failed: %v", err)
	}

	fmt.Println(result)
}
