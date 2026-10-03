package insights

import (
	"context"
	"github.com/adel/nimbus/backend/internal/testutil"
	"github.com/google/uuid"
	"math"
	"testing"
	"time"
)

func TestRepositoryWindowAndTenantIsolation(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	testutil.ResetDatabase(t, db)
	ctx := context.Background()
	owner, other, app, foreign := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO users(id,name,email,password_hash) VALUES($1,'Owner','owner@insights.test','x'),($2,'Other','other@insights.test','x')`, owner, other)
	exec(`INSERT INTO applications(id,user_id,name,application_url,health_url) VALUES($1,$2,'API','https://example.com','https://example.com'),($3,$4,'Other','https://example.com','https://example.com')`, app, owner, foreign, other)
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	exec(`INSERT INTO health_checks(application_id,latency_ms,healthy,checked_at) VALUES($1,100,true,$3::timestamptz-interval '1 hour'),($1,200,true,$3::timestamptz-interval '50 minutes'),($1,9999,false,$3::timestamptz-interval '40 minutes'),($2,1,true,$3::timestamptz-interval '1 hour'),($1,1,true,$3::timestamptz-interval '25 hours')`, app, foreign, now)
	exec(`INSERT INTO deployments(application_id,version,previous_version,status,triggered_by,started_at,completed_at) VALUES($1,'v2','v1','successful',$2,$3::timestamptz-interval '8 seconds',$3)`, app, owner, now)
	exec(`INSERT INTO incidents(application_id,incident_type,title,severity,status,dedup_key,created_at,resolved_at) VALUES($1,'health_failure','Down','critical','resolved','insights-a',$3::timestamptz-interval '2 minutes',$3),($2,'health_failure','Private','critical','open','insights-b',$3,NULL)`, app, foreign, now)
	repo := Repository{DB: db}
	result, err := repo.Get(ctx, owner, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.Checks != 3 || result.Metrics.Successful != 2 {
		t.Fatalf("wrong counts: %+v", result.Metrics)
	}
	if result.Metrics.P95Latency == nil || math.Abs(*result.Metrics.P95Latency-195) > 0.001 {
		t.Fatal("p95 should include successful responses only")
	}
	if result.Metrics.AverageLatency == nil || *result.Metrics.AverageLatency != 150 {
		t.Fatal("wrong average")
	}
	if len(result.History) != 25 {
		t.Fatal("missing hourly buckets")
	}
	if len(result.Releases) != 1 || result.Releases[0].DurationSeconds == nil || *result.Releases[0].DurationSeconds != 8 {
		t.Fatal("wrong release duration")
	}
	if len(result.Incidents) != 1 || result.Incidents[0].DurationSeconds != 120 || result.ActiveIncidents != 0 {
		t.Fatal("wrong incident scope or duration")
	}
	empty, err := repo.Get(ctx, uuid.New(), now)
	if err != nil {
		t.Fatal(err)
	}
	if empty.Metrics.SuccessPercent != nil || empty.Metrics.AverageLatency != nil || empty.Metrics.P95Latency != nil || len(empty.Releases) != 0 || len(empty.Incidents) != 0 {
		t.Fatal("empty workspace must preserve missing data")
	}
}
