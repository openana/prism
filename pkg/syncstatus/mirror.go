package syncstatus

type Mirror struct {
	Name         string `json:"name"`
	Status       string `json:"status"`
	LastUpdate   int64  `json:"last_update_ts"`
	LastStarted  int64  `json:"last_started_ts"`
	LastEnded    int64  `json:"last_ended_ts"`
	NextSchedule int64  `json:"next_schedule_ts"`
	Upstream     string `json:"upstream"`
	Size         string `json:"size"`
}
