package collector

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

var assets = []string{"BTC", "ETH", "SOL", "XRP"}

type counts struct {
	written    int
	skipped    int
	recovered  int
	unresolved int
}

func Run() error {
	start, err := time.Parse("2006-01-02", defaultStart)
	if err != nil {
		return err
	}
	end := time.Now().UTC().Truncate(24 * time.Hour)
	if err := os.MkdirAll(filepath.Dir(defaultOutput), 0755); err != nil {
		return err
	}
	file, err := os.Create(defaultOutput)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := csv.NewWriter(file)
	if err := writer.Write(columns); err != nil {
		return err
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return err
	}

	client := newGammaClient()
	totalRows := 0
	for _, asset := range assets {
		fmt.Printf("collecting %s...\n", strings.ToLower(asset))
		result, err := collectAsset(client, asset, start, end, writer)
		if err != nil {
			return fmt.Errorf("%s: %w", strings.ToLower(asset), err)
		}
		totalRows += result.written
		fmt.Printf("%s complete: %d rows (%d gaps recovered, %d unresolved, %d skipped)\n",
			strings.ToLower(asset), result.written, result.recovered, result.unresolved, result.skipped)
	}

	fmt.Printf("wrote %d rows to %s\n", totalRows, defaultOutput)
	return nil
}

func collectAsset(client *gammaClient, asset string, start, end time.Time, writer *csv.Writer) (counts, error) {
	var result counts
	seenMarkets := make(map[string]bool)
	step := 5 * time.Minute
	today := time.Now().UTC().Truncate(24 * time.Hour)
	for batchStart := start; !batchStart.After(end); {
		batchEnd := batchStart.AddDate(0, 0, 6)
		if batchEnd.After(end) {
			batchEnd = end
		}
		batchRows := make(map[string]record)
		err := client.fetchSeries(series[asset], batchStart, batchEnd, func(e event) error {
			row, ok := eventToRecord(asset, series[asset], e)
			if !ok || len(row.endTime) < 10 || row.endTime[:10] < batchStart.Format("2006-01-02") || row.endTime[:10] > batchEnd.Format("2006-01-02") {
				result.skipped++
				return nil
			}
			batchRows[row.endTime] = row
			return nil
		})
		if err != nil {
			return result, err
		}

		lastSlot := batchStart.Add(-step)
		if batchEnd.Before(today) {
			lastSlot = batchEnd.Add(23*time.Hour + 55*time.Minute)
		} else if len(batchRows) > 0 {
			keys := make([]string, 0, len(batchRows))
			for key := range batchRows {
				keys = append(keys, key)
			}
			lastSlot, err = time.Parse(time.RFC3339Nano, slices.Max(keys))
			if err != nil {
				return result, err
			}
		}
		for slot := batchStart; !slot.After(lastSlot); slot = slot.Add(step) {
			slotISO := slot.Format(time.RFC3339)
			if _, found := batchRows[slotISO]; found {
				continue
			}
			slug := fmt.Sprintf("%s-updown-5m-%d", strings.ToLower(asset), slot.Add(-step).Unix())
			e, err := client.fetchBySlug(slug)
			if err != nil {
				return result, err
			}
			if e != nil {
				if row, ok := eventToRecord(asset, series[asset], *e); ok && row.endTime == slotISO {
					batchRows[slotISO] = row
					result.recovered++
					continue
				}
			}
			result.unresolved++
		}

		keys := make([]string, 0, len(batchRows))
		for key := range batchRows {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			row := batchRows[key]
			if seenMarkets[row.marketID] {
				continue
			}
			seenMarkets[row.marketID] = true
			if err := writer.Write(row.fields); err != nil {
				return result, err
			}
			result.written++
		}
		writer.Flush()
		if err := writer.Error(); err != nil {
			return result, err
		}
		fmt.Printf("%s: through %s, %d rows\n", strings.ToLower(asset), batchEnd.Format("2006-01-02"), result.written)
		batchStart = batchEnd.AddDate(0, 0, 1)
	}
	return result, nil
}
