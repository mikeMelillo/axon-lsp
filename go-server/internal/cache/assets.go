package cache

import _ "embed"

var (
	//go:embed assets/function_cache.json
	EmbeddedFunctionCache []byte

	//go:embed assets/coreFuncs.trio
	EmbeddedCoreFuncs []byte
)
