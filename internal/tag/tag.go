package tag

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	CreatedAtTag   = "created_at:"
	UserIDTag      = "user_id:"
	IDTag          = "id:"
	StoreMetricTag = "metric:store"
	GetMetricTag   = "metric:get"
	DeletedTag     = "deleted:true"
	TTLTag         = "ttl:"
)

func GetUserID(tags []string) (string, bool) {
	for _, t := range tags {
		if strings.Contains(t, UserIDTag) {
			return t, true
		}
	}
	return "no user id", false
}

func GetCreatedAt(tags []string) (time.Time, bool) {
	for _, t := range tags {
		if !strings.Contains(t, CreatedAtTag) {
			continue
		}
		ss := strings.Split(t, CreatedAtTag)
		if len(ss) < 2 {
			return time.Now().UTC(), false
		}
		tagTime, err := time.Parse(time.RFC3339Nano, ss[1])
		if err != nil {
			return time.Now().UTC(), false
		}
		return tagTime, true
	}
	return time.Now().UTC(), false
}

func HasAll(has []string, requested ...string) bool {
	reqMap := map[string]int{}
	for _, s := range requested {
		reqMap[s] = 1
	}
	for _, s := range has {
		reqMap[s] = 0
	}

	for _, count := range reqMap {
		if count > 0 {
			return false
		}
	}
	return true
}

func HasNone(has []string, mustNotHave ...string) bool {
	noMap := map[string]int{}
	for _, s := range mustNotHave {
		noMap[s] = 1
	}

	for _, s := range has {
		if noMap[s] > 0 {
			return false
		}
	}
	return true
}

func GetID(tags []string) (string, bool) {
	for _, t := range tags {
		if strings.Contains(t, IDTag) {
			return t, true
		}
	}
	return "no id", false
}

func NewCreatedAtTag(t time.Time) string {
	return fmt.Sprintf("created_at:%v", t.UTC().Format(time.RFC3339Nano))
}

func NewUpdatedAtTag(t time.Time) string {
	return fmt.Sprintf("updated_at:%v", time.Now().UTC().Format(time.RFC3339Nano))
}

func NewID() string {
	return fmt.Sprintf("id:%s", uuid.New().String())
}

func Q(has, mustHave, mustNotHave []string) bool {
	return HasNone(has, mustNotHave...) && HasAll(has, mustHave...)
}

func GetTTL(tags []string) (time.Duration, bool) {
	for _, t := range tags {
		if strings.Contains(t, TTLTag) {
			ss := strings.Split(t, "ttl:")
			if len(ss) < 2 {
				return 0, false
			}
			dur, err := time.ParseDuration(ss[1])
			if err != nil {
				return 0, false
			}
			return dur, true
		}
	}
	return 0, false
}

func GetChan(tags []string) string {
	for _, t := range tags {
		if strings.Contains(t, "chan:") {
			return strings.Split(t, "chan:")[1]
		}
	}
	return "general"
}
