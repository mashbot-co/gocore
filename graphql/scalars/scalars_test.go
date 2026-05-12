package scalars

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// --- UUID ---

func TestMarshalUUID_WritesQuotedString(t *testing.T) {
	u := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	var buf bytes.Buffer
	MarshalUUID(u).MarshalGQL(&buf)

	want := `"11111111-2222-3333-4444-555555555555"`
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUnmarshalUUID_AcceptsString(t *testing.T) {
	want := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	got, err := UnmarshalUUID(want.String())
	if err != nil {
		t.Fatalf("UnmarshalUUID: %v", err)
	}
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestUnmarshalUUID_RejectsMalformedString(t *testing.T) {
	if _, err := UnmarshalUUID("not-a-uuid"); err == nil {
		t.Fatal("expected error for malformed UUID")
	}
}

func TestUnmarshalUUID_RejectsNonString(t *testing.T) {
	got, err := UnmarshalUUID(42)
	if err == nil {
		t.Fatal("expected error for non-string input")
	}
	if got != uuid.Nil {
		t.Errorf("expected uuid.Nil on error, got %v", got)
	}
}

// --- DateTime ---

func TestMarshalDateTime_WritesRFC3339(t *testing.T) {
	tm := time.Date(2026, 5, 11, 12, 34, 56, 0, time.UTC)
	var buf bytes.Buffer
	MarshalDateTime(tm).MarshalGQL(&buf)

	got := buf.String()
	want := `"2026-05-11T12:34:56Z"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUnmarshalDateTime_AcceptsRFC3339String(t *testing.T) {
	got, err := UnmarshalDateTime("2026-05-11T12:34:56Z")
	if err != nil {
		t.Fatalf("UnmarshalDateTime: %v", err)
	}
	if got.Year() != 2026 || got.Month() != 5 || got.Day() != 11 {
		t.Errorf("parsed time mismatch: %v", got)
	}
}

func TestUnmarshalDateTime_RejectsBadFormat(t *testing.T) {
	if _, err := UnmarshalDateTime("yesterday"); err == nil {
		t.Fatal("expected error for non-RFC3339 input")
	}
}

func TestUnmarshalDateTime_RejectsNonString(t *testing.T) {
	got, err := UnmarshalDateTime(12345)
	if err == nil {
		t.Fatal("expected error for non-string input")
	}
	if !got.IsZero() {
		t.Errorf("expected zero time, got %v", got)
	}
}

// --- JSON ---

func TestMarshalJSON_WritesRawBytes(t *testing.T) {
	payload := json.RawMessage(`{"hello":"world"}`)
	var buf bytes.Buffer
	MarshalJSON(payload).MarshalGQL(&buf)

	if got := buf.String(); got != `{"hello":"world"}` {
		t.Errorf("got %q, want %q", got, `{"hello":"world"}`)
	}
}

func TestMarshalJSON_NilWritesLiteralNull(t *testing.T) {
	var buf bytes.Buffer
	MarshalJSON(nil).MarshalGQL(&buf)

	if got := buf.String(); got != "null" {
		t.Errorf("got %q, want %q", got, "null")
	}
}

func TestUnmarshalJSON_AcceptsString(t *testing.T) {
	got, err := UnmarshalJSON(`{"hello":"world"}`)
	if err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if string(got) != `{"hello":"world"}` {
		t.Errorf("got %q", string(got))
	}
}

func TestUnmarshalJSON_AcceptsObject(t *testing.T) {
	got, err := UnmarshalJSON(map[string]interface{}{"hello": "world"})
	if err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if !strings.Contains(string(got), `"hello":"world"`) {
		t.Errorf("got %q, expected hello:world payload", string(got))
	}
}

func TestUnmarshalJSON_AcceptsArray(t *testing.T) {
	got, err := UnmarshalJSON([]interface{}{"a", "b", "c"})
	if err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if string(got) != `["a","b","c"]` {
		t.Errorf("got %q, want %q", string(got), `["a","b","c"]`)
	}
}

func TestUnmarshalJSON_ObjectWithUnencodableValueErrors(t *testing.T) {
	// json.Marshal can't encode a channel — exercises the err branch.
	_, err := UnmarshalJSON(map[string]interface{}{"bad": make(chan int)})
	if err == nil {
		t.Fatal("expected error for unencodable map value")
	}
}

func TestUnmarshalJSON_RejectsUnsupportedType(t *testing.T) {
	if _, err := UnmarshalJSON(42); err == nil {
		t.Fatal("expected error for unsupported scalar type")
	}
}
