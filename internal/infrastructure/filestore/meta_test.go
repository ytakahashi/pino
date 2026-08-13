package filestore

import (
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/ytakahashi/pino/internal/application"
)

func TestFromMetaRejectsForeignValues(t *testing.T) {
	tests := []struct {
		name string
		m    application.Meta
	}{
		{"a string", "meta"},
		{"bytes", []byte("meta")},
		{"a time", time.Now()},
		{"a struct of the same shape", lookalike{hash: sha256.Sum256([]byte(`{"a":1}`))}},
		{"a pointer to meta rather than a value", &meta{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := fromMeta(tt.m); !errors.Is(err, errForeignMeta) {
				t.Errorf("fromMeta returned %v, want errForeignMeta", err)
			}
		})
	}
}

// TestFromMetaSeparatesAnUnknownFile covers the port allowing a nil Meta and
// giving it a meaning of its own. Folding it in with a Meta that is merely
// wrong would leave the save flow unable to tell "there is nothing to compare
// against" from "the caller carried the wrong value", which are answered
// differently: the first is an ordinary state for a document that never came
// from disk, the second is a bug.
func TestFromMetaSeparatesAnUnknownFile(t *testing.T) {
	_, err := fromMeta(nil)

	if !errors.Is(err, errNoMeta) {
		t.Errorf("fromMeta(nil) returned %v, want errNoMeta", err)
	}

	if errors.Is(err, errForeignMeta) {
		t.Error("fromMeta(nil) is reported as a meta from another store")
	}
}
