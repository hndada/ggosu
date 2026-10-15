package settings

import "encoding/json"

// Chat controls room messages and in-app alerts. Sounds are opt-in.
type Chat struct {
	Ticker       bool `json:"ticker"`
	AutoHide     bool `json:"autoHide"`
	MentionPopup bool `json:"mentionPopup"`
	MessagePopup bool `json:"messagePopup"`
	MentionSound bool `json:"mentionSound"`
	Instant      bool `json:"instant"`
}

func (c Config) ChatSettings() Chat {
	if c.Chat != nil {
		return *c.Chat
	}
	return Chat{AutoHide: true, MentionPopup: true, MessagePopup: true}
}

func (c *Chat) UnmarshalJSON(b []byte) error {
	type plain Chat
	v := plain((Config{}).ChatSettings())
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*c = Chat(v)
	return nil
}
