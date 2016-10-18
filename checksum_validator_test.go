package download

import (
	"strings"
	"testing"
)

func TestNewValidatorWithInvalidChecksum(t *testing.T) {
	_, err := newValidator(nil, nil, "totally invalid", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.HasPrefix(err.Error(), "invalid checksum") {
		t.Fatalf("wrong error returned, expected to start with '%s', received '%v'", "invalid checksum", err)
	}
}
