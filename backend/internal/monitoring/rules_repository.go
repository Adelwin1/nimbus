package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) LoadCheckRules(
	ctx context.Context,
	applicationID uuid.UUID,
) (CheckRules, error) {
	var rules CheckRules
	var expected []byte

	err := r.db.QueryRow(ctx, `
  SELECT expected_status,required_text,json_pointer,json_expected
  FROM application_check_rules WHERE application_id=$1
 `, applicationID).Scan(
		&rules.ExpectedStatus, &rules.RequiredText,
		&rules.JSONPointer, &expected,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return CheckRules{}, nil
	}
	if err != nil {
		return CheckRules{}, fmt.Errorf("load application check rules: %w", err)
	}
	rules.JSONExpected = json.RawMessage(expected)
	return rules, nil
}
