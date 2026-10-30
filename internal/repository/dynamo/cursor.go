package dynamo

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/roguekeyboards/kbdb/internal/repository"
)

// cursorEnvelope is the JSON payload base64-encoded into a cursor string.
// idx and pfx bind a cursor to the index and filter value it was minted
// under (the directory's username/discord prefix, the builds keyboard_id);
// List callers that take no filter leave both empty, and decoding always
// returns "" for them, so those paths behave exactly as before this field
// existed.
type cursorEnvelope struct {
	Key map[string]string `json:"k,omitempty"`
	Idx string            `json:"idx,omitempty"`
	Pfx string            `json:"pfx,omitempty"`
}

// decodeCursor reverses encodeCursor, returning a nil key (no key) for an
// empty cursor. idx/pfx are the index name and filter value the cursor
// was minted under, empty for callers with no filter.
func decodeCursor(cursor string) (key map[string]types.AttributeValue, idx, pfx string, err error) {
	if cursor == "" {
		return nil, "", "", nil //nolint:nilnil // no key is a valid, expected result
	}

	raw, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, "", "", fmt.Errorf("invalid cursor encoding: %w", err)
	}

	var env cursorEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, "", "", fmt.Errorf("invalid cursor contents: %w", err)
	}

	out := make(map[string]types.AttributeValue, len(env.Key))
	for k, v := range env.Key {
		out[k] = &types.AttributeValueMemberS{Value: v}
	}

	return out, env.Idx, env.Pfx, nil
}

// decodeOwnerCursor is decodeCursor for a List over ownerID's base-table
// partition: a cursor that doesn't decode, or whose key isn't exactly
// {user_id: ownerID, id}, is repository.ErrInvalidCursor rather than a
// start key DynamoDB rejects. pfx is the filter value the cursor was
// minted under.
func decodeOwnerCursor(cursor, ownerID string) (key map[string]types.AttributeValue, pfx string, err error) {
	key, _, pfx, err = decodeCursor(cursor)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %w", repository.ErrInvalidCursor, err)
	}
	if len(key) == 0 {
		return nil, pfx, nil
	}

	userID, ok := key["user_id"].(*types.AttributeValueMemberS)
	_, hasID := key["id"]
	if len(key) != 2 || !ok || userID.Value != ownerID || !hasID {
		return nil, "", fmt.Errorf("%w: key is not a position in this owner's list", repository.ErrInvalidCursor)
	}

	return key, pfx, nil
}

// encodeCursor is decodeCursor's inverse. Every LastEvaluatedKey paginated
// here is string-valued (base-table user_id/id and the directory GSI
// keys), so a plain map[string]string round-trips it without
// attributevalue's cursor-unfriendly type-tagged encoding. idx/pfx are
// stamped into the envelope so a future decodeCursor call can bind the
// cursor back to the filter it was minted under; pass "" for callers with
// no filter.
func encodeCursor(key map[string]types.AttributeValue, idx, pfx string) (string, error) {
	if len(key) == 0 {
		return "", nil
	}

	plain := make(map[string]string, len(key))
	for k, v := range key {
		s, ok := v.(*types.AttributeValueMemberS)
		if !ok {
			return "", fmt.Errorf("unexpected non-string attribute %q in last evaluated key", k)
		}
		plain[k] = s.Value
	}

	raw, err := json.Marshal(cursorEnvelope{Key: plain, Idx: idx, Pfx: pfx})
	if err != nil {
		return "", fmt.Errorf("marshalling cursor: %w", err)
	}

	return base64.URLEncoding.EncodeToString(raw), nil
}
