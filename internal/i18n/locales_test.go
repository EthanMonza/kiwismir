package i18n

import "testing"

func TestLocalesLoadAndHaveBackendKey(t *testing.T) {
	b, err := New("en")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, code := range b.Codes() {
		if got := b.T(code, "backend_unavailable"); got == "backend_unavailable" {
			t.Errorf("locale %q is missing backend_unavailable key", code)
		}
	}
	if got := b.T("en", "backend_unavailable"); got == "" {
		t.Error("en backend_unavailable is empty")
	}
}
