package collector

const (
	defaultStart  = "2026-02-25"
	defaultOutput = "data/polymarket_5m_resolutions.csv"
	pageSize      = 100
)

var series = map[string]string{
	"BTC": "10684",
	"ETH": "10683",
	"SOL": "10686",
	"XRP": "10685",
}

var columns = []string{
	"asset", "series_id", "event_id", "market_id", "slug", "title",
	"window_start_utc", "window_end_utc", "resolution", "up_price",
	"down_price", "uma_resolution_status", "volume", "volume_clob",
	"open_interest", "last_trade_price", "closed_time", "condition_id",
	"resolution_source",
}
