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

// FormatHeading38Card renders only position type, RTK condition, and correction age for the UI card.
func FormatHeading38Card(fields []gsof.Field) string {
	order := []struct {
		key      string
		outLabel string
	}{
		{"Position fix type", "Position type"},
		{"RTK condition", "RTK condition"},
		{"Correction age (s)", "Correction age (s)"},
	}
	var b strings.Builder
	byLabel := make(map[string]string, len(fields))
	for i := range fields {
		byLabel[fields[i].Label] = fields[i].Value
	}
	for _, o := range order {
		v, ok := byLabel[o.key]
		if !ok {
			continue
		}
		b.WriteString(o.outLabel)
		b.WriteString(": ")
		b.WriteString(v)
		b.WriteByte('\n')
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
