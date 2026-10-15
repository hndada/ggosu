package settings

import (
	"encoding/json"
	"testing"
)

func TestOldChatSettingsDefaults(t *testing.T) {
	var c Config
	if err := json.Unmarshal([]byte(`{"chat":{"ticker":true}}`), &c); err != nil {
		t.Fatal(err)
	}
	got := c.ChatSettings()
	if !got.Ticker || !got.AutoHide || !got.MentionPopup || !got.MessagePopup || got.MentionSound || got.Instant {
		t.Fatalf("partial chat defaults: %+v", got)
	}
	if err := json.Unmarshal([]byte(`{"chat":{"ticker":"true"}}`), &c); err == nil {
		t.Fatal("accepted a nonboolean setting")
	}
}
