package conformance

import (
	"testing"

	"github.com/tempest-io/tempest/internal/persistence/memstore"
)

func TestMemstoreConformance(t *testing.T) {
	store := memstore.New()
	RunAll(t, store)
}
