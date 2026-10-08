package workerdispatch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestDispatchOnlyTrustedPublicWorkflow(t *testing.T) {
	t.Setenv("NIMBUS_WORKER_MODE", "github-actions")
	t.Setenv("NIMBUS_ACTIONS_REPOSITORY", "Adelwin1/nimbus")
	t.Setenv("NIMBUS_ACTIONS_TOKEN", "test-secret")
	old := Client
	defer func() { Client = old }()
	for _, private := range []bool{false, true} {
		calls := 0
		Client = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.Host != "api.github.com" || r.Header.Get("Authorization") != "Bearer test-secret" {
				t.Fatal("incorrect destination/auth")
			}
			body := `{"private":false,"full_name":"Adelwin1/nimbus"}`
			if private {
				body = `{"private":true,"full_name":"Adelwin1/nimbus"}`
			}
			status := 200
			if r.Method == "POST" {
				var p struct {
					Ref    string
					Inputs map[string]string
				}
				if json.NewDecoder(r.Body).Decode(&p) != nil || p.Ref != "main" || p.Inputs["kind"] != "repository" || len(p.Inputs) != 2 {
					t.Fatal("bad dispatch payload")
				}
				status = 204
				body = ""
			}
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}
		err := Dispatch(context.Background(), "repository", uuid.New())
		if private {
			if err == nil || calls != 1 {
				t.Fatal("private repo dispatched")
			}
		} else if err != nil || calls != 2 {
			t.Fatal("public dispatch failed")
		}
	}
}
func TestLocalModeDoesNotDispatch(t *testing.T) {
	t.Setenv("NIMBUS_WORKER_MODE", "")
	if Dispatch(context.Background(), "browser", uuid.New()) != nil {
		t.Fatal("local dispatch")
	}
}
func TestRejectInvalidDispatch(t *testing.T) {
	t.Setenv("NIMBUS_WORKER_MODE", "github-actions")
	t.Setenv("NIMBUS_ACTIONS_REPOSITORY", "owner/repo/extra")
	t.Setenv("NIMBUS_ACTIONS_TOKEN", "secret")
	if Dispatch(context.Background(), "browser", uuid.New()) == nil {
		t.Fatal("invalid destination")
	}
}
