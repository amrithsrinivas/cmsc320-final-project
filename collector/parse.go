package collector

import (
	"encoding/json"
	"strconv"
	"strings"
)

type event struct {
	ID           string      `json:"id"`
	Slug         string      `json:"slug"`
	Title        string      `json:"title"`
	StartTime    string      `json:"startTime"`
	EndDate      string      `json:"endDate"`
	Closed       bool        `json:"closed"`
	OpenInterest json.Number `json:"openInterest"`
	Markets      []market    `json:"markets"`
}

type market struct {
	ID                  string      `json:"id"`
	Closed              bool        `json:"closed"`
	UMAResolutionStatus string      `json:"umaResolutionStatus"`
	Outcomes            string      `json:"outcomes"`
	OutcomePrices       string      `json:"outcomePrices"`
	EventStartTime      string      `json:"eventStartTime"`
	EndDate             string      `json:"endDate"`
	VolumeNum           json.Number `json:"volumeNum"`
	VolumeClob          json.Number `json:"volumeClob"`
	LastTradePrice      json.Number `json:"lastTradePrice"`
	ClosedTime          string      `json:"closedTime"`
	ConditionID         string      `json:"conditionId"`
	ResolutionSource    string      `json:"resolutionSource"`
}

type record struct {
	fields   []string
	endTime  string
	marketID string
}

func eventToRecord(asset, seriesID string, e event) (record, bool) {
	if !e.Closed || len(e.Markets) != 1 {
		return record{}, false
	}
	m := e.Markets[0]
	if !m.Closed || !strings.EqualFold(m.UMAResolutionStatus, "resolved") {
		return record{}, false
	}
	var labels []string
	var priceStrings []json.Number
	if json.Unmarshal([]byte(m.Outcomes), &labels) != nil ||
		json.Unmarshal([]byte(m.OutcomePrices), &priceStrings) != nil ||
		len(labels) != 2 || labels[0] != "Up" || labels[1] != "Down" || len(priceStrings) != 2 {
		return record{}, false
	}
	up, upErr := strconv.ParseFloat(string(priceStrings[0]), 64)
	down, downErr := strconv.ParseFloat(string(priceStrings[1]), 64)
	if upErr != nil || downErr != nil {
		return record{}, false
	}
	resolution := ""
	switch {
	case up == 1 && down == 0:
		resolution = "Up"
	case up == 0 && down == 1:
		resolution = "Down"
	case up == 0.5 && down == 0.5:
		resolution = "Draw5050"
	default:
		return record{}, false
	}
	startTime := m.EventStartTime
	if startTime == "" {
		startTime = e.StartTime
	}
	endTime := m.EndDate
	if endTime == "" {
		endTime = e.EndDate
	}
	fields := []string{
		asset, seriesID, e.ID, m.ID, e.Slug, e.Title, startTime, endTime,
		resolution, strconv.FormatFloat(up, 'f', 1, 64), strconv.FormatFloat(down, 'f', 1, 64),
		m.UMAResolutionStatus, string(m.VolumeNum), string(m.VolumeClob),
		string(e.OpenInterest), string(m.LastTradePrice), m.ClosedTime,
		m.ConditionID, m.ResolutionSource,
	}
	return record{fields: fields, endTime: endTime, marketID: m.ID}, true
}
