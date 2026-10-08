package workerdispatch

import (
	"context"
	"errors"
	"testing"

	"github.com/adel/nimbus/backend/internal/testutil"
	"github.com/google/uuid"
)

func TestHostedQuotaAndRollback(t *testing.T) {
	t.Setenv("NIMBUS_WORKER_MODE", "github-actions")
	db := testutil.OpenTestDatabase(t)
	testutil.ResetDatabase(t, db)
	ctx := context.Background()
	user := uuid.New()
	if _, err := db.Exec(ctx, `INSERT INTO users(id,name,email,password_hash) VALUES($1,'Quota','quota@example.com','test')`, user); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = Reserve(ctx, tx, user, uuid.New(), "browser"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if db.QueryRow(ctx, `SELECT count(*) FROM hosted_worker_dispatches`).Scan(&count) != nil || count != 0 {
		t.Fatal("rolled back job consumed quota")
	}
	for i := 0; i < 10; i++ {
		tx, err = db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err = Reserve(ctx, tx, user, uuid.New(), "repository"); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if !errors.Is(Reserve(ctx, tx, user, uuid.New(), "browser"), ErrQuota) {
		t.Fatal("user quota not enforced across both kinds")
	}
}
