package providers

import "encoding/json"

// json* aliases allow tests and the mock to call these without importing encoding/json.
var (
	jsonMarshal   = json.Marshal
	jsonUnmarshal = json.Unmarshal
)
