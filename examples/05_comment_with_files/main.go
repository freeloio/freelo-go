// Post a comment with file attachments.
//
// Two-step flow:
//  1. Upload the file via /file/upload (multipart) — server returns a UUID.
//  2. Post the comment with `files: [{"uuid": "..."}]`.
//
// The OpenAPI spec models attachments differently from what the live
// server accepts; the {"uuid": "..."} shape is what works in practice.
// We use *WithBody to escape the typed body, since the typed model can't
// represent that shape.
//
//	FREELO_TASK_ID=12345 FREELO_FILE=./README.md go run ./examples/05_comment_with_files
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"os"
	"strconv"

	"github.com/freeloio/freelo-go"
	"github.com/freeloio/freelo-go/auth"
)

type uploadResp struct {
	UUID     string `json:"uuid"`
	Filename string `json:"filename"`
}

func main() {
	taskID, err := strconv.Atoi(os.Getenv("FREELO_TASK_ID"))
	if err != nil || taskID == 0 {
		log.Fatal("set FREELO_TASK_ID")
	}
	filePath := os.Getenv("FREELO_FILE")
	if filePath == "" {
		log.Fatal("set FREELO_FILE to a file you want attached")
	}

	client, err := freelo.New(
		freelo.WithAuth(auth.BasicAuth{
			Email:  os.Getenv("FREELO_EMAIL"),
			APIKey: os.Getenv("FREELO_API_KEY"),
		}),
		freelo.WithUserAgent("freelo-go-comment-with-files/0.1"),
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	// Step 1: build the multipart body and upload via the typed *WithBody
	// helper (which accepts a Content-Type, unlike Client.Raw).
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", filePath)
	if err != nil {
		log.Fatal(err)
	}
	f, err := os.Open(filePath)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := io.Copy(part, f); err != nil {
		log.Fatal(err)
	}
	_ = f.Close()
	_ = mw.Close()

	uploadResponse, err := client.API.UploadFileWithBody(ctx, mw.FormDataContentType(), &buf)
	if err != nil {
		log.Fatal(err)
	}
	defer uploadResponse.Body.Close()

	if uploadResponse.StatusCode >= 400 {
		body, _ := io.ReadAll(uploadResponse.Body)
		log.Fatalf("upload failed: %s — %s", uploadResponse.Status, body)
	}

	var up uploadResp
	if err := json.NewDecoder(uploadResponse.Body).Decode(&up); err != nil {
		log.Fatalf("decode upload response: %v", err)
	}
	fmt.Printf("uploaded: %s (%s)\n", up.Filename, up.UUID)

	// Step 2: post the comment using *WithBody — the typed
	// CreateCommentJSONRequestBody can't represent the {"uuid": ...} shape.
	commentBody, _ := json.Marshal(map[string]any{
		"content": fmt.Sprintf("See attached: %s", up.Filename),
		"files":   []map[string]string{{"uuid": up.UUID}},
	})
	resp, err := client.API.CreateCommentWithBody(ctx, taskID, "application/json", bytes.NewReader(commentBody))
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	fmt.Println("comment posted:", string(out))
}
