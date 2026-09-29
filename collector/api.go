package collector

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type gammaClient struct {
	baseURL string
	http    *http.Client
}

func newGammaClient() *gammaClient {
	return &gammaClient{
		baseURL: "https://gamma-api.polymarket.com",
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func retryDelay(header string, fallback time.Duration) time.Duration {
	if seconds, err := strconv.ParseFloat(header, 64); err == nil && seconds > 0 {
		return max(fallback, min(time.Duration(seconds*float64(time.Second)), 60*time.Second))
	}
	if when, err := http.ParseTime(header); err == nil {
		return max(fallback, min(time.Until(when), 60*time.Second))
	}
	return fallback
}

func (g *gammaClient) getJSON(path string, params url.Values, out any, allow404 bool) (bool, error) {
	backoff := time.Second
	for attempt := 0; attempt < 7; attempt++ {
		endpoint := g.baseURL + path
		if params != nil {
			endpoint += "?" + params.Encode()
		}
		resp, err := g.http.Get(endpoint)
		retryAfter := ""
		if err == nil {
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
			resp.Body.Close()
			if readErr != nil {
				err = readErr
			} else if resp.StatusCode == http.StatusOK {
				if jsonErr := json.Unmarshal(body, out); jsonErr != nil {
					return false, fmt.Errorf("invalid gamma json at %s: %w", path, jsonErr)
				}
				return true, nil
			} else if allow404 && resp.StatusCode == http.StatusNotFound {
				return false, nil
			} else {
				err = fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body[:min(len(body), 200)])))
				if resp.StatusCode != 429 && (resp.StatusCode < 500 || resp.StatusCode > 504) {
					return false, fmt.Errorf("gamma request %s: %w", path, err)
				}
				retryAfter = resp.Header.Get("Retry-After")
			}
		}
		if attempt == 6 {
			return false, fmt.Errorf("gamma request %s failed after 7 attempts: %w", path, err)
		}
		time.Sleep(retryDelay(retryAfter, backoff))
		backoff = min(backoff*2, 30*time.Second)
	}
	return false, errors.New("unreachable retry state")
}

func (g *gammaClient) fetchSeries(seriesID string, start, end time.Time, visit func(event) error) error {
	params := url.Values{
		"series_id":    {seriesID},
		"closed":       {"true"},
		"limit":        {strconv.Itoa(pageSize)},
		"order":        {"endDate"},
		"ascending":    {"true"},
		"end_date_min": {start.Format("2006-01-02") + "T00:00:00Z"},
		"end_date_max": {end.Format("2006-01-02") + "T23:59:59Z"},
	}
	seenCursors := make(map[string]bool)
	for {
		var page struct {
			Events     []event `json:"events"`
			NextCursor string  `json:"next_cursor"`
		}
		_, err := g.getJSON("/events/keyset", params, &page, false)
		if err != nil {
			return err
		}
		if page.Events == nil {
			return fmt.Errorf("unexpected gamma events response for series %s", seriesID)
		}
		for _, e := range page.Events {
			if err := visit(e); err != nil {
				return err
			}
		}
		if page.NextCursor == "" {
			return nil
		}
		if len(page.Events) == 0 || seenCursors[page.NextCursor] {
			return fmt.Errorf("gamma pagination stalled for series %s", seriesID)
		}
		seenCursors[page.NextCursor] = true
		params.Set("after_cursor", page.NextCursor)
		time.Sleep(125 * time.Millisecond)
	}
}

func (g *gammaClient) fetchBySlug(slug string) (*event, error) {
	var e event
	found, err := g.getJSON("/events/slug/"+url.PathEscape(slug), nil, &e, true)
	if err != nil || !found {
		return nil, err
	}
	return &e, nil
}
