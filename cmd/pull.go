package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

type ExportReply struct {
	Data struct {
		FileOperation struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"fileOperation"`
	} `json:"data"`
}

type FileOpInfo struct {
	Data struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		State     string `json:"state"`
		CreatedAt string `json:"createdAt"`
		UpdatedAt string `json:"updatedAt"`
	} `json:"data"`
}

type ExportCheck struct {
	Pagination struct {
		Total float64 `json:"total"`
	} `json:"pagination"`
	Data []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		State       string `json:"state"`
		CreatedAt   string `json:"createdAt"`
	} `json:"data"`
}

type Collection struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Desc string `json:"desc"`
}

var (
	includePrivate     bool
	includeAttachments bool
	overrideURL        string
	overrideToken      string
	forceExport        bool
	exportCooldown     int
	onlyCollection     bool
)

var pullCmd = &cobra.Command{
	Use:   "pull [PATH]",
	Short: "Export Outline collections",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// variables
		destFolder := args[0]
		token := overrideToken
		if token == "" {
			token = os.Getenv("OUTLINE_TOKEN")
		}

		baseURL := overrideURL
		if baseURL == "" {
			baseURL = os.Getenv("OUTLINE_URL")
		}

		if token == "" || baseURL == "" {
			log.Fatal("OUTLINE_TOKEN and OUTLINE_URL must be set")
		}

		client := &http.Client{}
		operationID := ""

		if onlyCollection {
			fetchCollectionsMetadata(client, baseURL, token, destFolder)
			return
		}

		// check for existing exports within exportCooldown
		if !forceExport {
			fmt.Printf("[INFO] Checking for existing exports (last %v min(s))...\n", exportCooldown)
			operationID = getRecentExportID(client, baseURL, token)
		}

		// start export if none exists
		if operationID == "" {
			operationID = triggerExport(client, baseURL, token)
			fmt.Printf("[OK]   Started a new export: %s\n", operationID)
			// check fileOperations.info for progress
			pollProgress(client, baseURL, token, operationID)
		}
		// download the file once it's ready
		downloadArchive(client, baseURL, token, operationID, destFolder)

		// fetch collection overview descriptions
		fetchCollectionsMetadata(client, baseURL, token, destFolder)
	},
}

func init() {
	rootCmd.AddCommand(pullCmd)
	pullCmd.Flags().BoolVar(&includeAttachments, "attachments", false, "include attachments (i.e. images and files)")
	pullCmd.Flags().BoolVar(&includePrivate, "private", false, "include private collections")
	pullCmd.Flags().BoolVar(&forceExport, "force", false, "force an export")
	pullCmd.Flags().IntVar(&exportCooldown, "cooldown", 10, "reuse an export within this time, if available (in mins)")
	pullCmd.Flags().BoolVar(&onlyCollection, "only-collections", false, "only pull metadata from collections (i.e. for their description)")
	pullCmd.Flags().StringVar(&overrideURL, "url", "", "Outline URL, overrides env")
	pullCmd.Flags().StringVar(&overrideToken, "token", "", "Outline Token, overrides env")
}

// checks for existing exports within exportCooldown
func getRecentExportID(client *http.Client, baseURL, token string) string {
	apiEndpoint, err := url.JoinPath(baseURL, "api/fileOperations.list")
	if err != nil {
		log.Fatalf("Failed to construct URL: %v", err)
	}

	payload := map[string]any{
		"sort": "createdAt",
		"type": "export",
	}
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		log.Fatalf("Failed to marshal payload: %v", err)
	}

	req, err := http.NewRequest("POST", apiEndpoint, bytes.NewBuffer(jsonBody))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Failed to create request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to read body: %v", err)
	}
	// fmt.Printf("\n%v\n", string(body))

	var reply ExportCheck
	err = json.Unmarshal(body, &reply)
	if err != nil {
		log.Fatalf("Error on Unmarshal ExportCheck JSON: %v", err)
	}

	// get the latest export if there is one
	if reply.Pagination.Total > 0 {
		latestTime, err := time.Parse(time.RFC3339Nano, reply.Data[0].CreatedAt)
		if err != nil {
			fmt.Printf("Error parsing latestTime: %v\n", err)
		}
		diff := time.Since(latestTime)

		// FIXME: will fail if latest export isn't "status": "complete"
		if diff < time.Minute*time.Duration(exportCooldown) {
			operationID := reply.Data[0].ID
			fmt.Printf("[OK]   Reusing export: %s\n", operationID)
			return operationID
		}
	}

	fmt.Printf("[INFO] No pre-existing exports found\n")
	return ""
}

func triggerExport(client *http.Client, baseURL, token string) string {
	apiEndpoint, err := url.JoinPath(baseURL, "/api/collections.export_all")
	if err != nil {
		log.Fatalf("Failed to construct URL: %v", err)
	}

	payload := map[string]any{
		"format":             "outline-markdown",
		"includeAttachments": includeAttachments,
		"includePrivate":     includePrivate,
	}
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		log.Fatalf("Failed to marshal payload: %v", err)
	}

	req, err := http.NewRequest("POST", apiEndpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		log.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+token)

	// start export
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Network error: %v", err)
	} else if resp.StatusCode != 200 {
		log.Fatalf("Error with response: %v\n", resp)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)

	var reply ExportReply
	err = json.Unmarshal(body, &reply)
	if err != nil {
		log.Fatalf("Failed to parse JSON: %v", err)
	}

	operationID := reply.Data.FileOperation.ID
	return operationID
}

func pollProgress(client *http.Client, baseURL, token, operationID string) {
	apiEndpoint, _ := url.JoinPath(baseURL, "/api/fileOperations.info")

	payload := map[string]string{"id": operationID}
	jsonBody, _ := json.Marshal(payload)

	fmt.Println("[INFO] Polling for completion... (every 3s)")

	for {
		req, _ := http.NewRequest("POST", apiEndpoint, bytes.NewBuffer(jsonBody))
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Add("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			log.Fatalf("Network error during polling: %v", err)
		}

		body, _ := io.ReadAll(resp.Body)
		defer func() { _ = resp.Body.Close() }()

		var reply FileOpInfo
		err = json.Unmarshal(body, &reply)
		if err != nil {
			log.Fatalf("Failed to parse JSON: %v", err)
		}

		state := reply.Data.State

		fmt.Printf("[INFO] Current state: %v\n", state)

		if state == "complete" {
			start, _ := time.Parse(time.RFC3339Nano, reply.Data.CreatedAt)
			end, _ := time.Parse(time.RFC3339Nano, reply.Data.UpdatedAt)
			duration := end.Sub(start).Round(time.Millisecond)

			fmt.Printf("[OK]   Export is ready (%s)\n", duration)
			break
		} else if state == "failed" || state == "error" {
			log.Fatalf("Export failed, your storage configuration could be wrong or are you rate-limited?: %+v", string(body))
		}

		// check up every 3 seconds
		time.Sleep(3 * time.Second)
	}
}

func downloadArchive(client *http.Client, baseURL, token, operationID, destFolder string) {
	fmt.Printf("[INFO] Downloading export archive...\n")
	start := time.Now()

	apiEndpoint, _ := url.JoinPath(baseURL, "/api/fileOperations.redirect")
	payload := map[string]string{"id": operationID}
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		log.Fatalf("Error making the payload: %v\n", err)
	}

	req, _ := http.NewRequest("POST", apiEndpoint, bytes.NewBuffer(jsonBody))
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Add("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Network error: %v\n", err)
	}

	if resp.StatusCode != 200 {
		log.Fatalf("Failed to download file, status code: %d\n", resp.StatusCode)
	}

	err = os.MkdirAll(destFolder, 0o755)
	if err != nil {
		log.Fatalf("Error creating the directory: %v\n", err)
	}

	timestamp := time.Now().Format("2006-01-02_15-04-05")
	fileName := fmt.Sprintf("export_%s.zip", timestamp)
	fullPath := filepath.Join(destFolder, fileName)

	outFile, err := os.Create(fullPath)
	if err != nil {
		log.Fatalf("Error creating the file: %v\n", err)
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		log.Fatalf("Error saving file: %v\n", err)
	}
	duration := time.Since(start).Round(time.Millisecond)

	// TODO: include file disk size
	fmt.Printf("[OK]   Saved to %s (%s)\n", fullPath, duration)
}

// separate request to get text in collections overview
// TODO: probably doesn't need to run when reusing an export
func fetchCollectionsMetadata(client *http.Client, baseURL, token, destFolder string) {
	fmt.Printf("[INFO] Fetching collections metadata...\n")
	start := time.Now()

	apiEndpoint, err := url.JoinPath(baseURL, "/api/collections.list")
	// FIXME: could run into issues if more than 100 collections
	payload := map[string]any{
		"direction": "DESC",
		"limit":     100,
	}
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		log.Fatalf("Error during body marshal: %v\n", err)
	}

	req, _ := http.NewRequest("POST", apiEndpoint, bytes.NewBuffer(jsonBody))
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Add("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Error during http Request: %v\n", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)

	// TODO: should this use it's own struct?
	var reply ExportCheck
	err = json.Unmarshal(body, &reply)
	if err != nil {
		log.Fatalf("Error during body unmarshal: %v\n", err)
	}

	// store it in collections.json
	// [ { "name": "collection", "desc": "description" }, { "name": "collection", "desc": "description" } ]
	var collections []Collection
	nameCounts := make(map[string]int)

	for _, item := range reply.Data {
		name := item.Name
		count := nameCounts[item.Name]
		if count > 0 {
			name = fmt.Sprintf("%s (%d)", item.Name, count)
		}
		nameCounts[item.Name]++

		collections = append(collections,
			Collection{
				ID:   item.ID,
				Name: name,
				Desc: item.Description,
			})
	}

	jsonData, err := json.Marshal(collections)
	if err != nil {
		log.Fatalf("Error during marshal: %v\n", err)
	}
	path := filepath.Join(destFolder, "collections.json")
	err = os.WriteFile(path, jsonData, 0o644)
	if err != nil {
		log.Fatalf("Error during collections write: %v\n", err)
	}

	duration := time.Since(start).Round(time.Millisecond)
	fmt.Printf("[OK]   Saved %d collections to %s (%s)\n", len(collections), path, duration)
}
