package mirrors

type Mirror struct {
	Name       string      `json:"name"`
	Metadata   *Metadata   `json:"metadata,omitempty"`
	SyncStatus *SyncStatus `json:"sync,omitempty"`
}

type Metadata struct {
	// "rsync", "proxy", "git"
	Type    string `json:"type"`
	Desc    string `json:"desc"`
	URL     string `json:"url"`
	HelpURL string `json:"help_url"`
}

type SyncStatus struct {
	Status       string `json:"status"`
	LastUpdate   int64  `json:"last_update"`
	LastStarted  int64  `json:"last_started"`
	LastEnded    int64  `json:"last_ended"`
	NextSchedule int64  `json:"next_schedule"`
	Upstream     string `json:"upstream"`
	Size         int64  `json:"size"`
}
