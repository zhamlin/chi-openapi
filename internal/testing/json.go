package testing

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/nsf/jsondiff"
)

func JSON(obj interface{}) (string, error) {
	b, err := json.MarshalIndent(&obj, "", " ")
	return string(b), err
}

func JSONT(t *testing.T, obj interface{}) string {
	val, err := JSON(obj)
	if err != nil {
		t.Error(err)
	}
	return val
}

func JSONDiff(t *testing.T, obj interface{}, data string) error {
	val, err := JSON(obj)
	if err != nil {
		return err
	}

	opts := jsondiff.DefaultConsoleOptions()
	diff, show := jsondiff.Compare([]byte(data), []byte(val), &opts)
	if diff.String() != "FullMatch" {
		return fmt.Errorf("%v:\n%v", diff, show)
	}
	return nil
}
