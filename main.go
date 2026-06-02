package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

func init() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
}

func main() {
	var dnsServer string
	flag.StringVar(&dnsServer, "dns", "", "DNS server to use for resolution (e.g. 8.8.8.8)")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "usage: ocr [--dns <server>] <image-path>\n")
		os.Exit(1)
	}
	imagePath := flag.Arg(0)

	var transport http.RoundTripper
	if dnsServer != "" {
		r := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{}
				return d.DialContext(ctx, "udp", net.JoinHostPort(dnsServer, "53"))
			},
		}
		dialer := &net.Dialer{Resolver: r}
		transport = &http.Transport{
			DialContext: dialer.DialContext,
		}
	}

	client := &http.Client{
		Timeout:   120 * time.Second,
		Transport: transport,
	}
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
