package gsofbaseline

import (
	"strings"

	"timing-analyzer/internal/gsof"
)

// FormatGSOFFieldsForCard renders Decode() rows as indented plain text for UI cards.
func FormatGSOFFieldsForCard(fields []gsof.Field) string {
	var b strings.Builder
	for i := range fields {
		formatFieldLine(&b, &fields[i], 0)
	}
	return strings.TrimSpace(b.String())
}

func formatFieldLine(b *strings.Builder, f *gsof.Field, depth int) {
	if f == nil {
		return
	}
	pad := strings.Repeat("  ", depth)
	b.WriteString(pad)
	b.WriteString(f.Label)
	b.WriteString(": ")
	b.WriteString(f.Value)
	b.WriteByte('\n')
	for j := range f.Detail {
		formatFieldLine(b, &f.Detail[j], depth+1)
	}
}
