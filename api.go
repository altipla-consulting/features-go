package features

type flagReply struct {
	Code    string       `json:"code"`
	Enabled bool         `json:"enabled"`
	Tenants []flagTenant `json:"tenants"`
}

type flagTenant struct {
	Code    string `json:"code"`
	Enabled bool   `json:"enabled"`
}

type statsRequest struct {
	Project string      `json:"project"`
	Stats   []statEntry `json:"stats"`
}

type statEntry struct {
	Bucket      int64  `json:"bucket"`
	Flag        string `json:"flag"`
	EnabledHits int64  `json:"enabledHits"`
	TotalHits   int64  `json:"totalHits"`
}
