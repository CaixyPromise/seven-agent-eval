package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"seven-agent-eval/internal/core"
	"strings"
	"testing"
)

func TestPreparedDownloadIsSingleUseAndCleansDisk(t *testing.T) {
	app, server := setup(t)
	r, err := app.Engine.Prepare(context.Background(), "smoke")
	if err != nil {
		t.Fatal(err)
	}
	r, err = app.Engine.Execute(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Post(server.URL+"/api/runs/"+r.ID+"/export", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	var ticket map[string]string
	if err = json.NewDecoder(response.Body).Decode(&ticket); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal(response.StatusCode, ticket)
	}
	token := strings.TrimPrefix(ticket["url"], "/api/downloads/")
	app.downloadMu.Lock()
	dir := app.downloads[token].dir
	app.downloadMu.Unlock()
	response, err = http.Get(server.URL + ticket["url"])
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
	if _, err = core.Import(bytes.NewReader(body), int64(len(body))); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("download staging directory remains")
	}
	response, err = http.Get(server.URL + ticket["url"])
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 404 {
		t.Fatal("ticket reused", response.StatusCode)
	}
	response, err = http.Get(server.URL + "/api/runs/" + r.ID + "/cases?limit=1")
	if err != nil {
		t.Fatal(err)
	}
	var page core.CasePage
	json.NewDecoder(response.Body).Decode(&page)
	response.Body.Close()
	if page.Total != r.Completed || len(page.Items) != 1 {
		t.Fatal("page mismatch")
	}
	response, err = http.Get(server.URL + "/api/runs/" + r.ID + "/cases/detail?case_id=missing")
	if err != nil {
		t.Fatal(err)
	}
	var failure map[string]string
	json.NewDecoder(response.Body).Decode(&failure)
	response.Body.Close()
	if failure["code"] != "CASE_NOT_FOUND" {
		t.Fatal("unsafe error classification", failure)
	}
}

func TestUnclaimedDownloadRemovedAtShutdown(t *testing.T) {
	app, server := setup(t)
	r, err := app.Engine.Prepare(context.Background(), "smoke")
	if err != nil {
		t.Fatal(err)
	}
	r, err = app.Engine.Execute(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Post(server.URL+"/api/runs/"+r.ID+"/export", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	var ticket map[string]string
	json.NewDecoder(response.Body).Decode(&ticket)
	response.Body.Close()
	token := strings.TrimPrefix(ticket["url"], "/api/downloads/")
	app.downloadMu.Lock()
	dir := app.downloads[token].dir
	app.downloadMu.Unlock()
	app.closeDownloads()
	if _, err = os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("unclaimed export retained")
	}
}
