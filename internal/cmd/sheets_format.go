package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SheetsFormatCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range         string `arg:"" name:"range" help:"Range (eg. Sheet1!A1:B2)"`
	FormatJSON    string `name:"format-json" help:"Cell format as JSON (Sheets API CellFormat)"`
	FormatFields  string `name:"format-fields" help:"Format field mask (eg. userEnteredFormat.textFormat.bold)"`
}

func (c *SheetsFormatCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	spreadsheetID := strings.TrimSpace(c.SpreadsheetID)
	rangeSpec := cleanRange(c.Range)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if strings.TrimSpace(rangeSpec) == "" {
		return usage("empty range")
	}
	if strings.TrimSpace(c.FormatJSON) == "" {
		return fmt.Errorf("provide format JSON via --format-json")
	}
	formatFields := strings.TrimSpace(c.FormatFields)
	if formatFields == "" {
		return fmt.Errorf("provide format fields via --format-fields")
	}

	var format sheets.CellFormat
	if err := json.Unmarshal([]byte(c.FormatJSON), &format); err != nil {
		return fmt.Errorf("invalid format JSON: %w", err)
	}

	rangeInfo, err := parseA1Range(rangeSpec)
	if err != nil {
		return err
	}
	if strings.TrimSpace(rangeInfo.SheetName) == "" {
		return fmt.Errorf("format range must include a sheet name")
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	sheetIDs, err := fetchSheetIDMap(ctx, svc, spreadsheetID)
	if err != nil {
		return err
	}
	sheetID, ok := sheetIDs[rangeInfo.SheetName]
	if !ok {
		return fmt.Errorf("unknown sheet %q in format range", rangeInfo.SheetName)
	}

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				RepeatCell: &sheets.RepeatCellRequest{
					Range: toGridRange(rangeInfo, sheetID),
					Cell: &sheets.CellData{
						UserEnteredFormat: &format,
					},
					Fields: formatFields,
				},
			},
		},
	}

	if _, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"range":  rangeSpec,
			"fields": formatFields,
		})
	}

	u.Out().Printf("Formatted %s", rangeSpec)
	return nil
}
