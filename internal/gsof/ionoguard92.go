package gsof

import "fmt"

// decodeIonoGuard92 decodes GSOF type 0x5C (92) IonoGuard info.
// Field layout is defined in the linked specification; until wired here, the payload is shown as hex.
func decodeIonoGuard92(payload []byte) []Field {
	return []Field{
		kv("Summary", Lookup(92).Function),
		kv("Payload length (bytes)", fmt.Sprintf("%d", len(payload))),
		kv("Payload (hex)", hexPreview(payload, 96)),
		kv("Note", "Structured decode not implemented yet; see the type 92 specification at Doc URL."),
	}
}

// decodeIonoGuard96 decodes GSOF type 0x60 (96) IonoGuard summary.
// Field layout is defined in the linked specification; until wired here, the payload is shown as hex.
func decodeIonoGuard96(payload []byte) []Field {
	return []Field{
		kv("Summary", Lookup(96).Function),
		kv("Payload length (bytes)", fmt.Sprintf("%d", len(payload))),
		kv("Payload (hex)", hexPreview(payload, 96)),
		kv("Note", "Structured decode not implemented yet; see the type 96 specification at Doc URL."),
	}
}
