package operation_setting

import "testing"

func TestAllowsCacheHitBoost(t *testing.T) {
	s := &QuotaSetting{CacheHitBoostChannelIds: "12, 34;56\n78"}
	if s.AllowsCacheHitBoost(0) {
		t.Fatal("channel 0 must be rejected")
	}
	if s.AllowsCacheHitBoost(99) {
		t.Fatal("id outside list must be rejected")
	}
	for _, id := range []int{12, 34, 56, 78} {
		if !s.AllowsCacheHitBoost(id) {
			t.Fatalf("id %d should be allowlisted", id)
		}
	}
	empty := &QuotaSetting{CacheHitBoostChannelIds: ""}
	if empty.AllowsCacheHitBoost(12) {
		t.Fatal("empty list must boost nobody")
	}
}
