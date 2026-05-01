// Create a task in a tasklist. The endpoint requires both the project ID
// and the tasklist ID.
//
//	FREELO_PROJECT_ID=580898 FREELO_TASKLIST_ID=12345 go run ./examples/04_create_task
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/freeloio/freelo-go"
	"github.com/freeloio/freelo-go/auth"
	"github.com/freeloio/freelo-go/freeloapi"
)

func main() {
	projectID, err := strconv.Atoi(os.Getenv("FREELO_PROJECT_ID"))
	if err != nil || projectID == 0 {
		log.Fatal("set FREELO_PROJECT_ID")
	}
	tasklistID, err := strconv.Atoi(os.Getenv("FREELO_TASKLIST_ID"))
	if err != nil || tasklistID == 0 {
		log.Fatal("set FREELO_TASKLIST_ID")
	}

	client, err := freelo.New(
		freelo.WithAuth(auth.BasicAuth{
			Email:  os.Getenv("FREELO_EMAIL"),
			APIKey: os.Getenv("FREELO_API_KEY"),
		}),
		freelo.WithUserAgent("freelo-go-create-task/0.1"),
	)
	if err != nil {
		log.Fatal(err)
	}

	body := freeloapi.CreateTaskJSONRequestBody{
		Name: "Created from freelo-go example",
	}

	resp, err := client.API.CreateTaskWithResponse(context.Background(), projectID, tasklistID, body)
	if err != nil {
		log.Fatal(err)
	}
	if resp.StatusCode() >= 400 {
		log.Fatalf("create failed: %s — %s", resp.Status(), string(resp.Body))
	}

	fmt.Printf("created task: %s\n", string(resp.Body))
}
