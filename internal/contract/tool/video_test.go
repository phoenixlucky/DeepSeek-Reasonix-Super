package tool

import (
	"context"
	"testing"
)

func TestTightestVideoLimitNamesTheSourceThatBinds(t *testing.T) {
	n := NativeVideo{Protocol: "inline_data_url", Limits: []VideoLimit{
		{Source: "inline_data_url", Bytes: 7_500_000},
		{Source: "tools.video_native_max_bytes", Bytes: 5_000_000},
		{Source: "other", Bytes: 9_000_000},
	}}
	if got := n.Tightest(); got.Source != "tools.video_native_max_bytes" || got.Bytes != 5_000_000 {
		t.Fatalf("tightest = %+v", got)
	}
	if got := (NativeVideo{}).Tightest(); got.Bytes != 0 || got.Source != "" {
		t.Fatalf("no limits must read as none, not as zero bytes allowed: %+v", got)
	}
}

func TestNativeVideoTravelsWithTheCallContext(t *testing.T) {
	if _, ok := NativeVideoFrom(context.Background()); ok {
		t.Fatal("a bare context offers native video")
	}
	if _, ok := NativeVideoFrom(nil); ok { //nolint:staticcheck // the accessor tolerates a nil context
		t.Fatal("a nil context offers native video")
	}
	ctx := WithNativeVideo(context.Background(), func() (NativeVideo, bool) { return NativeVideo{Protocol: "moonshot_file"}, true })
	if got, ok := NativeVideoFrom(ctx); !ok || got.Protocol != "moonshot_file" {
		t.Fatalf("got %+v %v", got, ok)
	}
	off := WithNativeVideo(context.Background(), func() (NativeVideo, bool) { return NativeVideo{}, false })
	if _, ok := NativeVideoFrom(off); ok {
		t.Fatal("a source that declines must read as not offered")
	}
}
