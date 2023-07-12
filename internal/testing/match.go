package testing

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/nsf/jsondiff"
)

type Tester interface {
	Helper()
	Fatal(...any)
	Fatalf(string, ...any)
}

func JsonMustMatch(t Tester, input, expected string, messages ...any) {
	t.Helper()
	err := jsonDiff(input, expected)
	if err != nil {
		msg := messageFromAny("JsonMustMatch error", messages...)
		t.Fatalf("%s\n%s", msg, err.Error())
	}
}

func MustMatchAsJson(t Tester, input, expected any, messages ...any) {
	t.Helper()
	JsonMustMatch(t, MustMarshal(t, input), MustMarshal(t, expected), messages...)
}

func MustMatch(t Tester, a, b any, messages ...any) {
	t.Helper()
	if diff := cmp.Diff(a, b); diff != "" {
		msg := messageFromAny("MustMatch error", messages...)
		t.Fatalf("%s\n%s", msg, diff)
	}
}

func MustNotMatch(t *testing.T, a, b any, messages ...any) {
	t.Helper()
	if diff := cmp.Diff(a, b); diff == "" {
		msg := messageFromAny("MustNotMatch error", messages...)
		t.Fatalf("%s\nitems matched:\n\ta: %#v\n\tb: %#v", msg, a, b)
	}
}

func messageFromAny(defaultMessage string, messages ...any) string {
	msg := defaultMessage
	l := len(messages)
	if l == 1 {
		if stringMsg, ok := messages[0].(string); ok {
			msg = stringMsg
		}
	}
	if l >= 2 {
		if stringMsg, ok := messages[0].(string); ok {
			msg = fmt.Sprintf(stringMsg, messages[1:]...)
		}
	}
	return msg
}

func MustMarshal(t Tester, obj any) string {
	t.Helper()

	switch val := obj.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	}

	val, err := json.MarshalIndent(&obj, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	return string(val)
}

func jsonDiff(input, expected string) error {
	opts := jsondiff.DefaultConsoleOptions()
	diff, show := jsondiff.Compare([]byte(expected), []byte(input), &opts)
	if diff.String() != "FullMatch" {
		return fmt.Errorf("%v:\n%v", diff, show)
	}
	return nil
}
