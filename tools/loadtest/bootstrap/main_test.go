package main

import "testing"

func TestFindChannelAndToken(t *testing.T) {
	channels := []channelItem{
		{ID: 1, Name: "other"},
		{ID: 2, Name: defaultChannelName},
	}
	if got := findChannel(channels, defaultChannelName); got == nil || got.ID != 2 {
		t.Fatalf("expected to find channel id=2, got %+v", got)
	}
	if got := findChannel(channels, "missing"); got != nil {
		t.Fatalf("expected missing channel, got %+v", got)
	}

	tokens := []tokenItem{
		{ID: 11, Name: "other"},
		{ID: 12, Name: defaultTokenName},
	}
	if got := findToken(tokens, defaultTokenName); got == nil || got.ID != 12 {
		t.Fatalf("expected to find token id=12, got %+v", got)
	}
	if got := findToken(tokens, "missing"); got != nil {
		t.Fatalf("expected missing token, got %+v", got)
	}
}

func TestNormalizeAndFirstNonEmpty(t *testing.T) {
	if got := normalizeBaseURL(" http://127.0.0.1:3100/ "); got != "http://127.0.0.1:3100" {
		t.Fatalf("unexpected normalized URL: %q", got)
	}
	if got := firstNonEmpty("", "  ", "value", "other"); got != "value" {
		t.Fatalf("unexpected first non-empty value: %q", got)
	}
}
