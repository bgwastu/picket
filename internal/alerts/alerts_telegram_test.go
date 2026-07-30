package alerts

import "testing"

func TestNonEmptyTelegramUserIDs(t *testing.T) {
	got := NonEmptyTelegramUserIDs([]string{" 767305817 ", "", "  ", "42"})
	if len(got) != 2 || got[0] != "767305817" || got[1] != "42" {
		t.Fatalf("unexpected user IDs: %#v", got)
	}
}
