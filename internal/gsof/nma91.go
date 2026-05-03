package gsof

import "fmt"

// decodeNMA91 decodes GSOF type 0x5B (91) Navigation Message Authentication (NMA) info.
// Field layout is defined in the linked specification; until wired here, the payload is shown as hex.
func decodeNMA91(payload []byte) []Field {
	return []Field{
		kv("Summary", Lookup(91).Function),
		kv("Payload length (bytes)", fmt.Sprintf("%d", len(payload))),
		kv("Payload (hex)", hexPreview(payload, 96)),
		kv("Note", "Structured decode not implemented yet; see the type 91 specification at Doc URL."),
	}
}
