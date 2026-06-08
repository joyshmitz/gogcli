package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type linksSetRecorder struct {
	requests []map[string]any
}

func linksSetHandler(recorder *linksSetRecorder) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")

		// Metadata GET → resolve sheet name → ID.
		if strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet && !strings.Contains(path, "batchUpdate") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "s1",
				"sheets": []map[string]any{
					{"properties": map[string]any{"sheetId": 0, "title": "Sheet1"}},
				},
			})
			return
		}

		// batchUpdate POST → record requests.
		if strings.HasPrefix(path, "/spreadsheets/s1:batchUpdate") && r.Method == http.MethodPost {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			reqs, _ := body["requests"].([]any)
			recorder.requests = recorder.requests[:0]
			for _, rq := range reqs {
				if m, ok := rq.(map[string]any); ok {
					recorder.requests = append(recorder.requests, m)
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"spreadsheetId": "s1"})
			return
		}

		http.Error(w, "unexpected "+r.Method+" "+path, http.StatusNotFound)
	})
}

func newLinksSetService(t *testing.T, recorder *linksSetRecorder) *sheets.Service {
	t.Helper()
	srv := httptest.NewServer(linksSetHandler(recorder))
	t.Cleanup(srv.Close)
	svc, err := sheets.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func linksSetCtx(t *testing.T) context.Context {
	t.Helper()
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	return outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
}

// updateCellsCell digs the single CellData map out of recorded request index i.
func updateCellsCell(t *testing.T, rec *linksSetRecorder, i int) map[string]any {
	t.Helper()
	uc, ok := rec.requests[i]["updateCells"].(map[string]any)
	if !ok {
		t.Fatalf("request %d not updateCells: %#v", i, rec.requests[i])
	}
	rows := uc["rows"].([]any)
	values := rows[0].(map[string]any)["values"].([]any)
	return values[0].(map[string]any)
}

func cellRuns(t *testing.T, cell map[string]any) []any {
	t.Helper()
	runs, ok := cell["textFormatRuns"].([]any)
	if !ok {
		t.Fatalf("no textFormatRuns: %#v", cell)
	}
	return runs
}

func runLink(run any) (start float64, uri string) {
	m := run.(map[string]any)
	if s, ok := m["startIndex"].(float64); ok {
		start = s
	}
	if fmtM, ok := m["format"].(map[string]any); ok {
		if link, ok := fmtM["link"].(map[string]any); ok {
			uri, _ = link["uri"].(string)
		}
	}
	return start, uri
}

func TestSheetsLinksSet_SingleLink(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })
	rec := &linksSetRecorder{}
	svc := newLinksSetService(t, rec)
	newSheetsService = func(context.Context, string) (*sheets.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		if err := runKong(t, &SheetsLinksSetCmd{}, []string{"s1", "Sheet1!B2", "https://x.test/a", "Act A"}, linksSetCtx(t), flags); err != nil {
			t.Fatalf("links set: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	if result["cellsUpdated"] != float64(1) {
		t.Fatalf("cellsUpdated = %v", result["cellsUpdated"])
	}
	if len(rec.requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(rec.requests))
	}
	cell := updateCellsCell(t, rec, 0)
	if got := cell["userEnteredValue"].(map[string]any)["stringValue"]; got != "Act A" {
		t.Errorf("stringValue = %v, want Act A", got)
	}
	runs := cellRuns(t, cell)
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if _, uri := runLink(runs[0]); uri != "https://x.test/a" {
		t.Errorf("run uri = %q", uri)
	}
	// targets B2 → rowIndex 1, columnIndex 1
	start := rec.requests[0]["updateCells"].(map[string]any)["start"].(map[string]any)
	if start["rowIndex"] != float64(1) || start["columnIndex"] != float64(1) {
		t.Errorf("start = %#v, want row1/col1", start)
	}
}

func TestSheetsLinksSet_ClearsStaleWholeCellLink(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })
	rec := &linksSetRecorder{}
	svc := newLinksSetService(t, rec)
	newSheetsService = func(context.Context, string) (*sheets.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	_ = captureStdout(t, func() {
		if err := runKong(t, &SheetsLinksSetCmd{}, []string{"s1", "Sheet1!B2", "https://x.test/a", "A"}, linksSetCtx(t), flags); err != nil {
			t.Fatalf("links set: %v", err)
		}
	})
	// The field mask must clear a pre-existing whole-cell link, else a replaced
	// cell keeps the stale URL and `links get` reports it (does not round-trip).
	fields := rec.requests[0]["updateCells"].(map[string]any)["fields"]
	if fields != "userEnteredValue,textFormatRuns,userEnteredFormat.textFormat.link" {
		t.Errorf("fields mask = %q, must clear userEnteredFormat.textFormat.link", fields)
	}
}

func TestSheetsLinksSet_DefaultTextIsURL(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })
	rec := &linksSetRecorder{}
	svc := newLinksSetService(t, rec)
	newSheetsService = func(context.Context, string) (*sheets.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	_ = captureStdout(t, func() {
		if err := runKong(t, &SheetsLinksSetCmd{}, []string{"s1", "Sheet1!A1", "https://only.url/"}, linksSetCtx(t), flags); err != nil {
			t.Fatalf("links set: %v", err)
		}
	})
	cell := updateCellsCell(t, rec, 0)
	if got := cell["userEnteredValue"].(map[string]any)["stringValue"]; got != "https://only.url/" {
		t.Errorf("stringValue = %v, want the url", got)
	}
}

func TestSheetsLinksSet_MultiLinkRuns(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })
	rec := &linksSetRecorder{}
	svc := newLinksSetService(t, rec)
	newSheetsService = func(context.Context, string) (*sheets.Service, error) { return svc, nil }

	runsJSON := `[{"text":"Act A","uri":"https://a"},{"text":" / "},{"text":"Act B","uri":"https://b"}]`
	flags := &RootFlags{Account: "a@b.com"}
	_ = captureStdout(t, func() {
		if err := runKong(t, &SheetsLinksSetCmd{}, []string{"s1", "Sheet1!C3", "--runs-json", runsJSON}, linksSetCtx(t), flags); err != nil {
			t.Fatalf("links set: %v", err)
		}
	})
	cell := updateCellsCell(t, rec, 0)
	if got := cell["userEnteredValue"].(map[string]any)["stringValue"]; got != "Act A / Act B" {
		t.Errorf("stringValue = %q, want concat", got)
	}
	runs := cellRuns(t, cell)
	if len(runs) != 3 {
		t.Fatalf("expected 3 runs, got %d", len(runs))
	}
	// Run boundaries: "Act A"=5 → " / "=3 → start 0,5,8 with links a, (none), b
	s0, u0 := runLink(runs[0])
	s1, u1 := runLink(runs[1])
	s2, u2 := runLink(runs[2])
	if s0 != 0 || u0 != "https://a" {
		t.Errorf("run0 = (%v,%q)", s0, u0)
	}
	if s1 != 5 || u1 != "" {
		t.Errorf("run1 = (%v,%q), want plain at 5", s1, u1)
	}
	if s2 != 8 || u2 != "https://b" {
		t.Errorf("run2 = (%v,%q)", s2, u2)
	}
}

func TestSheetsLinksSet_BatchCellsJSON(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })
	rec := &linksSetRecorder{}
	svc := newLinksSetService(t, rec)
	newSheetsService = func(context.Context, string) (*sheets.Service, error) { return svc, nil }

	cellsJSON := `[{"cell":"Sheet1!B2","url":"https://b2","text":"B2"},{"cell":"Sheet1!B3","runs":[{"text":"x","uri":"https://x"}]}]`
	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		if err := runKong(t, &SheetsLinksSetCmd{}, []string{"s1", "--cells-json", cellsJSON}, linksSetCtx(t), flags); err != nil {
			t.Fatalf("links set: %v", err)
		}
	})
	var result map[string]any
	_ = json.Unmarshal([]byte(out), &result)
	if result["cellsUpdated"] != float64(2) {
		t.Errorf("cellsUpdated = %v, want 2", result["cellsUpdated"])
	}
	if len(rec.requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(rec.requests))
	}
}

func TestSheetsLinksSet_DryRun(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })
	rec := &linksSetRecorder{}
	svc := newLinksSetService(t, rec)
	newSheetsService = func(context.Context, string) (*sheets.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com", DryRun: true}
	out := captureStdout(t, func() {
		err := (&SheetsLinksSetCmd{SpreadsheetID: "s1", Cell: "Sheet1!B2", URL: "https://x", Text: "L"}).Run(linksSetCtx(t), flags)
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 0 {
			t.Fatalf("expected dry-run exit 0, got %v", err)
		}
	})
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	if result["dry_run"] != true || result["op"] != "sheets.links.set" {
		t.Errorf("unexpected dry-run output: %#v", result)
	}
	if len(rec.requests) != 0 {
		t.Errorf("dry-run should not call batchUpdate, got %d requests", len(rec.requests))
	}
}

func TestSheetsLinksSet_RejectsMultiCellRange(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	err := (&SheetsLinksSetCmd{SpreadsheetID: "s1", Cell: "Sheet1!A1:B2", URL: "https://x"}).Run(linksSetCtx(t), flags)
	if err == nil || !strings.Contains(err.Error(), "single cell") {
		t.Fatalf("expected single-cell error, got %v", err)
	}
}

func TestSheetsLinksSet_RejectsMixedInput(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	err := (&SheetsLinksSetCmd{SpreadsheetID: "s1", Cell: "Sheet1!A1", CellsJSON: `[{"cell":"Sheet1!A1","url":"u"}]`}).Run(linksSetCtx(t), flags)
	if err == nil || !strings.Contains(err.Error(), "not both") {
		t.Fatalf("expected mixed-input error, got %v", err)
	}
}

func TestSheetsLinksSet_RequiresURLorRuns(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	err := (&SheetsLinksSetCmd{SpreadsheetID: "s1", Cell: "Sheet1!A1"}).Run(linksSetCtx(t), flags)
	if err == nil || !strings.Contains(err.Error(), "provide url") {
		t.Fatalf("expected url-required error, got %v", err)
	}
}
