package vault

import (
	"strings"
	"testing"
)

func TestParseFileValid(t *testing.T) {
	content := `API_KEY=ENC[AES256_GCM,data:abc,iv:def,tag:ghi,type:str]
DB_HOST=ENC[AES256_GCM,data:xyz,iv:uvw,tag:rst,type:str]
kv_age_recipient_0=age1abc123
kv_age_key=wrapped_key_data
kv_mac=ENC[AES256_GCM,data:mac,iv:maciv,tag:mactag,type:str]
kv_version=1
`

	ef, err := parseFile(content)
	if err != nil {
		t.Fatalf("parseFile: %v", err)
	}

	if len(ef.values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(ef.values))
	}
	if ef.values["API_KEY"] != "ENC[AES256_GCM,data:abc,iv:def,tag:ghi,type:str]" {
		t.Fatalf("unexpected API_KEY: %s", ef.values["API_KEY"])
	}
	if len(ef.recipients) != 1 || ef.recipients[0] != "age1abc123" {
		t.Fatalf("unexpected recipients: %v", ef.recipients)
	}
	if ef.wrappedDataKey != "wrapped_key_data" {
		t.Fatalf("unexpected wrappedDataKey: %s", ef.wrappedDataKey)
	}
	if ef.mac != "ENC[AES256_GCM,data:mac,iv:maciv,tag:mactag,type:str]" {
		t.Fatalf("unexpected mac: %s", ef.mac)
	}
	if ef.version != "1" {
		t.Fatalf("unexpected version: %s", ef.version)
	}
}

func TestParseFileMissingDataKey(t *testing.T) {
	content := `API_KEY=ENC[AES256_GCM,data:abc,iv:def,tag:ghi,type:str]
kv_mac=somemac
`
	_, err := parseFile(content)
	if err == nil {
		t.Fatal("expected error for missing data key")
	}
}

func TestParseFileSkipsComments(t *testing.T) {
	content := `# this is a comment
API_KEY=ENC[AES256_GCM,data:abc,iv:def,tag:ghi,type:str]
# another comment
kv_age_key=wrapped
kv_mac=mac
`
	ef, err := parseFile(content)
	if err != nil {
		t.Fatalf("parseFile: %v", err)
	}
	if len(ef.values) != 1 {
		t.Fatalf("expected 1 value, got %d", len(ef.values))
	}
}

func TestParseFileSkipsEmptyLines(t *testing.T) {
	content := `

API_KEY=ENC[AES256_GCM,data:abc,iv:def,tag:ghi,type:str]

kv_age_key=wrapped
kv_mac=mac

`
	ef, err := parseFile(content)
	if err != nil {
		t.Fatalf("parseFile: %v", err)
	}
	if len(ef.values) != 1 {
		t.Fatalf("expected 1 value, got %d", len(ef.values))
	}
}

func TestParseFileSkipsLinesWithoutEquals(t *testing.T) {
	content := `NOEQ
API_KEY=ENC[AES256_GCM,data:abc,iv:def,tag:ghi,type:str]
kv_age_key=wrapped
kv_mac=mac
`
	ef, err := parseFile(content)
	if err != nil {
		t.Fatalf("parseFile: %v", err)
	}
	if len(ef.values) != 1 {
		t.Fatalf("expected 1 value, got %d", len(ef.values))
	}
}

func TestParseFileMultipleRecipients(t *testing.T) {
	content := `kv_age_recipient_0=age1first
kv_age_recipient_1=age1second
kv_age_recipient_2=age1third
kv_age_key=wrapped
kv_mac=mac
`
	ef, err := parseFile(content)
	if err != nil {
		t.Fatalf("parseFile: %v", err)
	}
	if len(ef.recipients) != 3 {
		t.Fatalf("expected 3 recipients, got %d", len(ef.recipients))
	}
}

func TestParseFileUnescapesDataKey(t *testing.T) {
	content := `kv_age_key=line1\\nline2\\nline3
kv_mac=mac
`
	ef, err := parseFile(content)
	if err != nil {
		t.Fatalf("parseFile: %v", err)
	}
	if !strings.Contains(ef.wrappedDataKey, "\n") {
		t.Fatal("wrappedDataKey should have unescaped newlines")
	}
}

func TestParseFileMetaPrefixNotInValues(t *testing.T) {
	content := `NORMAL=val
kv_age_key=wrapped
kv_mac=mac
kv_version=1
kv_age_recipient_0=age1abc
`
	ef, err := parseFile(content)
	if err != nil {
		t.Fatalf("parseFile: %v", err)
	}
	if len(ef.values) != 1 {
		t.Fatalf("metadata should not appear in values, got %d", len(ef.values))
	}
	if _, ok := ef.values["NORMAL"]; !ok {
		t.Fatal("NORMAL should be in values")
	}
}

func TestEscapeUnescapeRoundtrip(t *testing.T) {
	tests := []string{
		"no newlines",
		"line1\nline2",
		"line1\nline2\nline3",
		"\n",
		"\n\n\n",
		"",
		"no backslashes here",
	}

	for _, tt := range tests {
		escaped := escapeLine(tt)
		if strings.Contains(escaped, "\n") {
			t.Fatalf("escaped should not contain real newlines: %q", escaped)
		}
		unescaped := unescapeLine(escaped)
		if unescaped != tt {
			t.Fatalf("roundtrip failed: got %q, want %q", unescaped, tt)
		}
	}
}

func TestParseFileValueWithEquals(t *testing.T) {
	content := `URL=ENC[AES256_GCM,data:abc=def==,iv:ghi,tag:jkl,type:str]
kv_age_key=wrapped
kv_mac=mac
`
	ef, err := parseFile(content)
	if err != nil {
		t.Fatalf("parseFile: %v", err)
	}
	if ef.values["URL"] != "ENC[AES256_GCM,data:abc=def==,iv:ghi,tag:jkl,type:str]" {
		t.Fatalf("value with equals not preserved: %s", ef.values["URL"])
	}
}

func TestParseFileEmptyValues(t *testing.T) {
	content := `kv_age_key=wrapped
kv_mac=mac
`
	ef, err := parseFile(content)
	if err != nil {
		t.Fatalf("parseFile: %v", err)
	}
	if len(ef.values) != 0 {
		t.Fatalf("expected 0 values, got %d", len(ef.values))
	}
}
