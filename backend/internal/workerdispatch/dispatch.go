// Package workerdispatch starts bounded, on-demand checks in a trusted public repository.
package workerdispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrUnavailable = errors.New("Hosted checks unavailable. Check workflow configuration.")
var ErrQuota = errors.New("Hosted check limit reached. Try again tomorrow.")
var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
var Client = &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func Hosted() bool { return os.Getenv("NIMBUS_WORKER_MODE") == "github-actions" }

// Reserve runs in the same transaction as queue insertion, so quotas cannot race.
func Reserve(ctx context.Context, tx pgx.Tx, user, job uuid.UUID, kind string) error {
	if !Hosted() {
		return nil
	}
	if kind != "browser" && kind != "repository" {
		return ErrUnavailable
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('nimbus-hosted-quota',0))`); err != nil {
		return ErrUnavailable
	}
	var total, own int
	if err := tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE user_id=$1) FROM hosted_worker_dispatches WHERE created_at>now()-interval '24 hours'`, user).Scan(&total, &own); err != nil {
		return ErrUnavailable
	}
	if total >= 100 || own >= 10 {
		return ErrQuota
	}
	_, err := tx.Exec(ctx, `INSERT INTO hosted_worker_dispatches(job_id,user_id,kind) VALUES($1,$2,$3)`, job, user, kind)
	if err != nil {
		return ErrUnavailable
	}
	return nil
}

func Dispatch(ctx context.Context, kind string, job uuid.UUID) error {
	if !Hosted() {
		return nil
	}
	repo, token := os.Getenv("NIMBUS_ACTIONS_REPOSITORY"), os.Getenv("NIMBUS_ACTIONS_TOKEN")
	if !repositoryPattern.MatchString(repo) || token == "" || job == uuid.Nil || (kind != "browser" && kind != "repository") {
		return ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	// Refuse a private execution repository: private runner minutes can incur charges.
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/"+repo, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "Nimbus")
	req.Header.Set("Accept", "application/vnd.github+json")
	response, err := Client.Do(req)
	if err != nil {
		return ErrUnavailable
	}
	var visibility struct {
		Private  *bool  `json:"private"`
		FullName string `json:"full_name"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(response.Body, 65536)).Decode(&visibility)
	response.Body.Close()
	if response.StatusCode != 200 || decodeErr != nil || visibility.Private == nil || *visibility.Private || visibility.FullName != repo {
		return ErrUnavailable
	}
	payload, _ := json.Marshal(map[string]any{"ref": "main", "inputs": map[string]string{"kind": kind, "job_id": job.String()}})
	req, _ = http.NewRequestWithContext(ctx, "POST", "https://api.github.com/repos/"+repo+"/actions/workflows/nimbus-hosted-check.yml/dispatches", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "Nimbus")
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	response, err = Client.Do(req)
	if err != nil {
		return ErrUnavailable
	}
	response.Body.Close()
	if response.StatusCode != 204 && response.StatusCode != 200 {
		return ErrUnavailable
	}
	return nil
}
