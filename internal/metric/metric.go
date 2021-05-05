package metric

import (
	"encoding/json"
	"time"
)

type Metric struct {
	Start time.Time     `json:"start"`
	End   time.Time     `json:"end"`
	Dur   time.Duration `json:"duration"`
}

func (m Metric) Buf() []byte {
	b, _ := json.Marshal(m)
	return b
}
