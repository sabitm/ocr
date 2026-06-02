package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

var gradioURLs = []string{
	"https://paddlepaddle-paddleocr-vl-1-6-online-demo.hf.space/gradio_api",
	"https://paddlepaddle-paddleocr-vl-1-6-online-demo.ms.show/gradio_api",
}
var gradioRR atomic.Uint64

func debugf(format string, args ...any) {
	if os.Getenv("DEBUG") != "" {
		log.Printf(format, args...)
	}
}

func upload(client *http.Client, baseURL, localPath string) (string, error) {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	debugf("upload: read %d bytes from %s", len(data), localPath)

	boundary := fmt.Sprintf("----GoGradio%016x", rand.Int63())
	var body bytes.Buffer

	body.WriteString("--" + boundary + "\r\n")
	origName := localPath
	if idx := strings.LastIndex(origName, "/"); idx >= 0 {
		origName = origName[idx+1:]
	}
	body.WriteString(fmt.Sprintf("Content-Disposition: form-data; name=\"files\"; filename=\"%s\"\r\n", origName))
	body.WriteString("Content-Type: application/octet-stream\r\n\r\n")
	body.Write(data)
	body.WriteString("\r\n--" + boundary + "--\r\n")

	req, err := http.NewRequestWithContext(context.Background(), "POST", baseURL+"/upload", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	debugf("upload: POST %s/upload (%d bytes)", baseURL, body.Len())

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload failed (status %d): %s", resp.StatusCode, string(b))
	}

	var paths []string
	if err := json.NewDecoder(resp.Body).Decode(&paths); err != nil {
		return "", fmt.Errorf("parse upload response: %w", err)
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("upload returned no file paths")
	}
	debugf("upload: got server path=%q", paths[0])

	return paths[0], nil
}

func queuePredict(client *http.Client, baseURL, serverPath, origPath string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	sessionHash := fmt.Sprintf("%016x", rand.Int63())
	origName := origPath
	if idx := strings.LastIndex(origName, "/"); idx >= 0 {
		origName = origName[idx+1:]
	}

	payload, _ := json.Marshal(map[string]any{
		"data": []any{
			map[string]any{
				"path":      serverPath,
				"orig_name": origName,
				"meta":      map[string]any{"_type": "gradio.FileData"},
			},
			nil,
			false, false, false,
		},
		"fn_index":     2,
		"session_hash": sessionHash,
	})
	debugf("queueJoin: payload=%s", string(payload))

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/queue/join", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return "", fmt.Errorf("queue join failed (status %d): %s", resp.StatusCode, string(b))
	}

	var joinResp struct {
		EventID string `json:"event_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&joinResp); err != nil {
		resp.Body.Close()
		return "", fmt.Errorf("queue join decode: %w", err)
	}
	resp.Body.Close()
	debugf("queueJoin: event_id=%s", joinResp.EventID)

	streamURL := fmt.Sprintf("%s/queue/data?session_hash=%s", baseURL, sessionHash)
	debugf("queueJoin: streaming from %s", streamURL)

	streamReq, err := http.NewRequestWithContext(ctx, "GET", streamURL, nil)
	if err != nil {
		return "", err
	}

	streamResp, err := client.Do(streamReq)
	if err != nil {
		return "", err
	}
	defer streamResp.Body.Close()

	if streamResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(streamResp.Body)
		return "", fmt.Errorf("stream failed (status %d): %s", streamResp.StatusCode, string(b))
	}

	scanner := bufio.NewScanner(streamResp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		raw := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if raw == "" {
			continue
		}
		debugf("SSE: %s", raw)

		var event struct {
			Msg     string `json:"msg"`
			Success bool   `json:"success"`
			Output  *struct {
				Data []json.RawMessage `json:"data"`
			} `json:"output"`
		}
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			debugf("SSE parse error: %v", err)
			continue
		}

		if event.Msg == "process_completed" {
			if !event.Success {
				return "", fmt.Errorf("server processing failed")
			}
			if event.Output == nil || len(event.Output.Data) == 0 {
				return "", fmt.Errorf("no output data")
			}
			var text string
			if err := json.Unmarshal(event.Output.Data[0], &text); err != nil {
				return string(event.Output.Data[0]), nil
			}
			return text, nil
		}
	}
	return "", fmt.Errorf("stream ended without completion")
}


