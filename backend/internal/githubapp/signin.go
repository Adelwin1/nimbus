package githubapp

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func validChallenge(value string) ([]byte, error) {
	if len(value) != 64 {
		return nil, errors.New("invalid challenge")
	}
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return nil, err
	}
	return decoded, nil
}
func (h *Handler) LoginStart(w http.ResponseWriter, r *http.Request) {
	if !h.configured() || h.Auth == nil {
		fail(w, 503, "GitHub sign-in is not configured.")
		return
	}
	challenge, err := validChallenge(r.URL.Query().Get("challenge"))
	if err != nil {
		fail(w, 400, "Start GitHub sign-in from Nimbus.")
		return
	}
	state, err := nonce()
	if err != nil {
		fail(w, 500, "Sign-in unavailable.")
		return
	}
	_, err = h.DB.Exec(r.Context(), `INSERT INTO github_oauth_states(state_hash,user_id,phase,expires_at,login_challenge) VALUES($1,NULL,'oauth',now()+interval '10 minutes',$2)`, hash(state), challenge)
	if err != nil {
		fail(w, 500, "Sign-in unavailable.")
		return
	}
	http.SetCookie(w, h.oauthCookie(state, 600))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	q := url.Values{"client_id": {h.ClientID}, "redirect_uri": {h.CallbackURL}, "state": {state}}
	http.Redirect(w, r, "https://github.com/login/oauth/authorize?"+q.Encode(), http.StatusSeeOther)
}

// The immutable GitHub numeric identity, not a mutable login or email, owns the workspace.
func (h *Handler) resolveIdentity(ctx context.Context, githubID int64, login string, existing *uuid.UUID) (uuid.UUID, error) {
	tx, err := h.DB.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('nimbus-github-identity-' || $1,0))`, strconv.FormatInt(githubID, 10))
	if err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	err = tx.QueryRow(ctx, `SELECT user_id FROM github_login_identities WHERE github_user_id=$1`, githubID).Scan(&id)
	if err == nil {
		if existing != nil && id != *existing {
			return uuid.Nil, errors.New("identity already linked")
		}
	} else if errors.Is(err, pgx.ErrNoRows) {
		rows, e := tx.Query(ctx, `SELECT user_id FROM github_authorizations WHERE github_user_id=$1`, githubID)
		if e != nil {
			return uuid.Nil, e
		}
		var candidates []uuid.UUID
		for rows.Next() {
			var candidate uuid.UUID
			if e = rows.Scan(&candidate); e != nil {
				rows.Close()
				return uuid.Nil, e
			}
			candidates = append(candidates, candidate)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return uuid.Nil, e
		}
		if len(candidates) > 1 {
			return uuid.Nil, errors.New("ambiguous legacy identity")
		}
		if existing != nil {
			id = *existing
			if len(candidates) == 1 && candidates[0] != id {
				return uuid.Nil, errors.New("identity already linked")
			}
		} else if len(candidates) == 1 {
			id = candidates[0]
		} else {
			id = uuid.New()
			name := login
			if len(name) > 120 {
				name = name[:120]
			}
			_, e = tx.Exec(ctx, `INSERT INTO users(id,name,email,password_hash) VALUES($1,$2,$3,'!github-only')`, id, name, "github-"+strconv.FormatInt(githubID, 10)+"@users.nimbus.invalid")
			if e != nil {
				return uuid.Nil, e
			}
		}
		_, err = tx.Exec(ctx, `INSERT INTO github_login_identities(github_user_id,user_id) VALUES($1,$2)`, githubID, id)
		if err != nil {
			return uuid.Nil, err
		}
	} else {
		return uuid.Nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}
func (h *Handler) finishLogin(w http.ResponseWriter, r *http.Request, githubID int64, login, token string, expires int64, challenge []byte) {
	if h.Auth == nil || len(challenge) != 32 {
		fail(w, 503, "GitHub sign-in unavailable.")
		return
	}
	id, err := h.resolveIdentity(r.Context(), githubID, login, nil)
	if err != nil {
		fail(w, 409, "GitHub identity could not be matched to a workspace. Use your existing Nimbus login to reconnect.")
		return
	}
	encrypted, err := h.Encryptor.Encrypt(token)
	if err != nil {
		fail(w, 500, "Sign-in unavailable.")
		return
	}
	code, err := nonce()
	if err != nil {
		fail(w, 500, "Sign-in unavailable.")
		return
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, "Sign-in unavailable.")
		return
	}
	defer tx.Rollback(r.Context())
	_, err = tx.Exec(r.Context(), `INSERT INTO github_authorizations(user_id,github_user_id,login,token_encrypted,expires_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(user_id) DO UPDATE SET github_user_id=EXCLUDED.github_user_id,login=EXCLUDED.login,token_encrypted=EXCLUDED.token_encrypted,expires_at=EXCLUDED.expires_at,connected_at=now()`, id, githubID, login, encrypted, time.Now().Add(time.Duration(expires)*time.Second))
	if err == nil {
		_, err = tx.Exec(r.Context(), `DELETE FROM github_login_exchanges WHERE expires_at<now()`)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO github_login_exchanges(code_hash,challenge,user_id,expires_at) VALUES($1,$2,$3,now()+interval '2 minutes')`, hash(code), challenge, id)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		fail(w, 500, "Sign-in unavailable.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, h.FrontendURL+"/auth/github?code="+url.QueryEscape(code), http.StatusSeeOther)
}
func (h *Handler) consumeExchange(ctx context.Context, code, verifier string) (uuid.UUID, error) {
	if len(code) != 43 || len(verifier) != 64 {
		return uuid.Nil, errors.New("invalid exchange")
	}
	if _, err := hex.DecodeString(verifier); err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	err := h.DB.QueryRow(ctx, `DELETE FROM github_login_exchanges WHERE code_hash=$1 AND challenge=$2 AND expires_at>now() RETURNING user_id`, hash(code), hash(verifier)).Scan(&id)
	return id, err
}
func (h *Handler) LoginExchange(w http.ResponseWriter, r *http.Request) {
	if h.Auth == nil {
		fail(w, 503, "GitHub sign-in unavailable.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var input struct {
		Code     string `json:"code"`
		Verifier string `json:"verifier"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
		fail(w, 400, "Invalid sign-in exchange.")
		return
	}
	id, err := h.consumeExchange(r.Context(), input.Code, input.Verifier)
	if err != nil {
		fail(w, 400, "Sign-in expired or belongs to another browser. Start again from Nimbus.")
		return
	}
	session, err := h.Auth.CreateExternalSession(r.Context(), id)
	if err != nil {
		fail(w, 500, "Sign-in unavailable. Try again.")
		return
	}
	reply(w, 200, session)
}
