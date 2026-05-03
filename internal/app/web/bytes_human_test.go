package web

import "testing"

func TestBytesHumanRu(t *testing.T) {
	if got := BytesHumanRu(0); got != "0 Б" {
		t.Fatalf("0: %q", got)
	}
	if got := BytesHumanRu(512); got != "512 Б" {
		t.Fatalf("512: %q", got)
	}
	if got := BytesHumanRu(2048); got != "2 КБ" {
		t.Fatalf("2048: %q", got)
	}
	if got := BytesHumanRu(2 * 1024 * 1024); got != "2 МБ" {
		t.Fatalf("2MiB: %q", got)
	}
	if got := BytesHumanRu(100 * 1024 * 1024 * 1024); got != "100 ГБ" {
		t.Fatalf("100GiB: %q", got)
	}
}
