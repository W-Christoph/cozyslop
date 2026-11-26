package diag

import (
	"context"
	"database/sql"
	"log/slog"
	"time"
)

// Summary is what happened between Since and Until, counted, for the
// admin page: a day of health samples alone is more entries than a page of
// Entries holds.
type Summary struct {
	Since    int64           `json:"since"` // unix ms
	Until    int64           `json:"until"`
	BucketMs int64           `json:"bucketMs"` // AniList and Samples are per bucket of this length
	Counts   Counts          `json:"counts"`
	LastBoot *Entry          `json:"lastBoot"` // the newest boot, also before Since
	Boots    []Entry         `json:"boots"`    // the boots from Since, newest first
	AniList  []AniListBucket `json:"anilist"`  // buckets with calls only
	Samples  []SampleBucket  `json:"samples"`  // buckets with samples only
}

type Counts struct {
	Errors         int   `json:"errors"`   // log records from Error up
	Warnings       int   `json:"warnings"` // log records at Warn
	Stalls         int   `json:"stalls"`
	Requests       int   `json:"requests"` // slow or failed
	FailedRequests int   `json:"failedRequests"`
	AniList        int   `json:"anilist"` // calls
	AniListFailed  int   `json:"anilistFailed"`
	AniListLimited int   `json:"anilistLimited"` // answered 429: too many
	AniListPeak    int   `json:"anilistPeak"`    // the most calls in one minute
	AniListSent    int64 `json:"anilistSentBytes"`
	AniListGot     int64 `json:"anilistReceivedBytes"`
	Boots          int   `json:"boots"`
	Crashes        int   `json:"crashes"` // boots after a run that did not shut down
}

type AniListBucket struct {
	Time    int64 `json:"time"` // the bucket's start, unix ms
	Calls   int   `json:"calls"`
	Failed  int   `json:"failed"`
	Limited int   `json:"limited"` // of those failed: 429
	Peak    int   `json:"peak"`    // the most calls in one minute
	Sent    int64 `json:"sentBytes"`
	Got     int64 `json:"receivedBytes"`
}

// SampleBucket is the highest of each health value in a bucket (the
// lowest of the machine's free memory); nil where no sample had it.
type SampleBucket struct {
	Time        int64    `json:"time"`
	CPU         *float64 `json:"cpuPct"`
	MemoryMB    *float64 `json:"memoryMb"`
	HostFreeMB  *float64 `json:"hostFreeMb"`
	HostTotalMB *float64 `json:"hostTotalMb"`
	People      *float64 `json:"people"`
	// The room that used the most memory, and its cap (hub.RoomMemory).
	RoomMemoryMB *float64 `json:"roomMemoryMb"`
	RoomLimitMB  *float64 `json:"roomLimitMb"`
	LagMs        *float64 `json:"lagMs"`
	LockMs       *float64 `json:"lockMs"`
	DBMs         *float64 `json:"dbMs"`
	RequestMs    *float64 `json:"requestMs"` // the oldest request under way
}

// SummaryBuckets is about how many buckets a Summary has.
const SummaryBuckets = 144

// Summarize counts what happened between since and until.
func (s *Store) Summarize(ctx context.Context, since, until time.Time) (Summary, error) {
	from, to := since.UnixMilli(), until.UnixMilli()
	bucket := max((to-from)/SummaryBuckets/60_000, 1) * 60_000 // whole minutes
	sum := Summary{Since: from, Until: to, BucketMs: bucket, Boots: []Entry{}, AniList: []AniListBucket{}, Samples: []SampleBucket{}}
	if s == nil {
		return sum, nil
	}
	boots, err := s.Entries(ctx, Filter{Kinds: []string{KindBoot}, Limit: 1})
	if err != nil {
		return sum, err
	}
	if len(boots) > 0 {
		sum.LastBoot = &boots[0]
	}
	if sum.Boots, err = s.Entries(ctx, Filter{Kinds: []string{KindBoot}, Since: since, Until: until, Limit: 100}); err != nil {
		return sum, err
	}

	c := &sum.Counts
	err = s.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(kind = 'event' AND level >= ?), 0),
		COALESCE(SUM(kind = 'event' AND level >= ? AND level < ?), 0),
		COALESCE(SUM(kind = 'stall' AND level >= ?), 0),
		COALESCE(SUM(kind = 'request'), 0),
		COALESCE(SUM(kind = 'request' AND json_extract(NULLIF(attrs, ''), '$.status') >= 500), 0),
		COALESCE(SUM(kind = 'outbound' AND json_extract(NULLIF(attrs, ''), '$.target') = 'anilist'), 0),
		COALESCE(SUM(kind = 'outbound' AND json_extract(NULLIF(attrs, ''), '$.target') = 'anilist' AND level >= ?), 0),
		COALESCE(SUM(kind = 'boot'), 0),
		COALESCE(SUM(kind = 'boot' AND json_extract(NULLIF(attrs, ''), '$.previous') IS NOT NULL), 0)
		FROM entries WHERE ts >= ? AND ts < ? AND kind != 'sample'`,
		slog.LevelError, slog.LevelWarn, slog.LevelError, slog.LevelError, slog.LevelWarn, from, to,
	).Scan(&c.Errors, &c.Warnings, &c.Stalls, &c.Requests, &c.FailedRequests, &c.AniList, &c.AniListFailed, &c.Boots, &c.Crashes)
	if err != nil {
		return sum, err
	}

	// AniList per minute, then per bucket with the busiest minute.
	rows, err := s.db.QueryContext(ctx, `SELECT ts / 60000, COUNT(*), SUM(level >= ?),
		COALESCE(SUM(json_extract(NULLIF(attrs, ''), '$.status') = 429), 0),
		COALESCE(SUM(json_extract(NULLIF(attrs, ''), '$.sent_bytes')), 0), COALESCE(SUM(json_extract(NULLIF(attrs, ''), '$.received_bytes')), 0)
		FROM entries
		WHERE kind = 'outbound' AND ts >= ? AND ts < ? AND json_extract(NULLIF(attrs, ''), '$.target') = 'anilist'
		GROUP BY 1 ORDER BY 1`, slog.LevelWarn, from, to)
	if err != nil {
		return sum, err
	}
	for rows.Next() {
		var minute, sent, got int64
		var calls, failed, limited int
		if err := rows.Scan(&minute, &calls, &failed, &limited, &sent, &got); err != nil {
			rows.Close()
			return sum, err
		}
		start := from + (minute*60_000-from)/bucket*bucket
		if n := len(sum.AniList); n == 0 || sum.AniList[n-1].Time != start {
			sum.AniList = append(sum.AniList, AniListBucket{Time: start})
		}
		b := &sum.AniList[len(sum.AniList)-1]
		b.Calls, b.Failed, b.Limited, b.Peak = b.Calls+calls, b.Failed+failed, b.Limited+limited, max(b.Peak, calls)
		b.Sent, b.Got = b.Sent+sent, b.Got+got
		c.AniListLimited, c.AniListPeak = c.AniListLimited+limited, max(c.AniListPeak, calls)
		c.AniListSent, c.AniListGot = c.AniListSent+sent, c.AniListGot+got
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return sum, err
	}

	rows, err = s.db.QueryContext(ctx, `SELECT (ts - ?) / ?,
		MAX(json_extract(NULLIF(attrs, ''), '$.cpu_pct')), MAX(json_extract(NULLIF(attrs, ''), '$.memory_mb')),
		MIN(json_extract(NULLIF(attrs, ''), '$.host_free_mb')), MAX(json_extract(NULLIF(attrs, ''), '$.host_total_mb')),
		MAX(json_extract(NULLIF(attrs, ''), '$.people')), MAX(json_extract(NULLIF(attrs, ''), '$.lag_ms')),
		MAX(json_extract(NULLIF(attrs, ''), '$.lock_max_ms')), MAX(json_extract(NULLIF(attrs, ''), '$.db_main_ms')),
		MAX(json_extract(NULLIF(attrs, ''), '$.oldest_request_ms')),
		MAX(json_extract(NULLIF(attrs, ''), '$.room_memory_mb')), MAX(json_extract(NULLIF(attrs, ''), '$.room_limit_mb'))
		FROM entries WHERE kind = 'sample' AND ts >= ? AND ts < ? GROUP BY 1 ORDER BY 1`,
		from, bucket, from, to)
	if err != nil {
		return sum, err
	}
	defer rows.Close()
	for rows.Next() {
		var i int64
		var v [11]sql.NullFloat64
		if err := rows.Scan(&i, &v[0], &v[1], &v[2], &v[3], &v[4], &v[5], &v[6], &v[7], &v[8], &v[9], &v[10]); err != nil {
			return sum, err
		}
		b := SampleBucket{Time: from + i*bucket}
		for j, p := range []**float64{&b.CPU, &b.MemoryMB, &b.HostFreeMB, &b.HostTotalMB, &b.People, &b.LagMs, &b.LockMs, &b.DBMs, &b.RequestMs, &b.RoomMemoryMB, &b.RoomLimitMB} {
			if v[j].Valid {
				*p = &v[j].Float64
			}
		}
		sum.Samples = append(sum.Samples, b)
	}
	return sum, rows.Err()
}
