package cache

import (
	"encoding/json"
	"fmt"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/typeinfo"
)

func LoadEmbeddedTypes() ([]typeinfo.Library, error) {
	var bundle typeinfo.Bundle
	if err := json.Unmarshal(EmbeddedTypeCatalog, &bundle); err != nil {
		return nil, fmt.Errorf("unmarshal embedded type catalog: %w", err)
	}
	if bundle.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported type catalog schema: %d", bundle.SchemaVersion)
	}
	return bundle.Libraries, nil
}
