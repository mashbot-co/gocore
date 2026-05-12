// Package scalars provides GraphQL scalar marshalers shared across all
// gocore-based APIs. Wired into a consumer's schema via gqlgen.yml:
//
//	models:
//	  UUID:
//	    model: github.com/mashbot-co/gocore/graphql/scalars.UUID
//	  DateTime:
//	    model: github.com/mashbot-co/gocore/graphql/scalars.DateTime
//	  JSON:
//	    model: github.com/mashbot-co/gocore/graphql/scalars.JSON
//
// gqlgen resolves the Marshal/Unmarshal function pair by name, so the
// exported functions below are the contract.
package scalars

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/google/uuid"
)

// MarshalUUID implements the graphql.Marshaler interface for uuid.UUID.
func MarshalUUID(u uuid.UUID) graphql.Marshaler {
	return graphql.WriterFunc(func(w io.Writer) {
		io.WriteString(w, `"`+u.String()+`"`)
	})
}

// UnmarshalUUID implements the graphql.Unmarshaler interface for uuid.UUID.
func UnmarshalUUID(v interface{}) (uuid.UUID, error) {
	switch v := v.(type) {
	case string:
		return uuid.Parse(v)
	default:
		return uuid.Nil, fmt.Errorf("uuid must be a string")
	}
}

// MarshalDateTime implements the graphql.Marshaler interface for time.Time.
func MarshalDateTime(t time.Time) graphql.Marshaler {
	return graphql.WriterFunc(func(w io.Writer) {
		io.WriteString(w, `"`+t.Format(time.RFC3339)+`"`)
	})
}

// UnmarshalDateTime implements the graphql.Unmarshaler interface for time.Time.
func UnmarshalDateTime(v interface{}) (time.Time, error) {
	switch v := v.(type) {
	case string:
		return time.Parse(time.RFC3339, v)
	default:
		return time.Time{}, fmt.Errorf("datetime must be a string in RFC3339 format")
	}
}

// MarshalJSON implements the graphql.Marshaler interface for json.RawMessage.
func MarshalJSON(j json.RawMessage) graphql.Marshaler {
	return graphql.WriterFunc(func(w io.Writer) {
		if j == nil {
			io.WriteString(w, "null")
			return
		}
		w.Write(j)
	})
}

// UnmarshalJSON implements the graphql.Unmarshaler interface for json.RawMessage.
func UnmarshalJSON(v interface{}) (json.RawMessage, error) {
	switch v := v.(type) {
	case string:
		return json.RawMessage(v), nil
	case map[string]interface{}, []interface{}:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		return json.RawMessage(b), nil
	default:
		return nil, fmt.Errorf("json must be a string or object")
	}
}
