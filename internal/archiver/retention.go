package archiver

import (
	"sort"
	"time"
)

// RetentionPolicy governs automated archiving and purging of workflow run records.
type RetentionPolicy struct {
	MaxAge         time.Duration
	KeepFailed     bool
	MinRetainCount int
}

// FilterForRetention splits records into retain and purge sets according to policy.
func FilterForRetention(records []ArchiveRecord, now time.Time, policy RetentionPolicy) (retain, purge []ArchiveRecord) {
	if len(records) == 0 {
		return nil, nil
	}

	// Sort chronologically newest first
	sorted := make([]ArchiveRecord, len(records))
	copy(sorted, records)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].FinishedAt.After(sorted[j].FinishedAt)
	})

	cutoff := now.Add(-policy.MaxAge)

	for i, r := range sorted {
		// Minimum retention protection (keep newest N regardless of age)
		if i < policy.MinRetainCount {
			retain = append(retain, r)
			continue
		}

		// Failed runs retention exception
		if policy.KeepFailed && r.FinalState == "FAILED" {
			retain = append(retain, r)
			continue
		}

		// Check age expiration
		if policy.MaxAge > 0 && r.FinishedAt.Before(cutoff) {
			purge = append(purge, r)
		} else {
			retain = append(retain, r)
		}
	}

	return retain, purge
}
