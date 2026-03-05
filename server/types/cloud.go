package types

type CloudListResponse struct {
	Path  string   `json:"path"`
	Items []string `json:"items"`
}

type CloudAuthResponse struct {
	URL string `json:"url"`
}
