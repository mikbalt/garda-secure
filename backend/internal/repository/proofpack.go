package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// IncidentCount is the number of incidents of one guard, on one day (site
// time), at one effective severity.
type IncidentCount struct {
	Day      string // YYYY-MM-DD in the site's time zone
	GuardID  string
	Severity string
	N        int
}

// IncidentCounts counts the incidents of a site that occurred in [from, to),
// by day, guard and effective severity (after amendments). Counts only: no
// incident text leaves the database (the monthly proof pack, D-074).
func IncidentCounts(ctx context.Context, db DBTX, siteID, tz string, from, to time.Time) ([]IncidentCount, error) {
	rows, err := db.Query(ctx, `
		SELECT to_char(occurred_at AT TIME ZONE $4, 'YYYY-MM-DD'), guard_id::text, eff_severity, COUNT(*)::int
		FROM incidents_effective
		WHERE site_id = $1 AND occurred_at >= $2 AND occurred_at < $3
		GROUP BY 1, 2, 3`, siteID, from, to, tz)
	if err != nil {
		return nil, fmt.Errorf("IncidentCounts: %w", err)
	}
	return collect(rows, func(r pgx.Rows) (IncidentCount, error) {
		var c IncidentCount
		err := r.Scan(&c.Day, &c.GuardID, &c.Severity, &c.N)
		return c, err
	})
}
