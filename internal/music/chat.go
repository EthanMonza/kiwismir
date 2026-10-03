package music

import (
	tele "gopkg.in/telebot.v3"
)

// sendStatus posts the "working..." status message for a new request. In
// groups it is threaded as a reply to the requester's message so a busy chat
// can tell which link is being fetched; in private chats it is a plain
// message, exactly as before.
func (h *Handler) sendStatus(c tele.Context, text string) (*tele.Message, error) {
	if m := c.Message(); m != nil && m.FromGroup() {
		return h.tb.Reply(m, text, htmlMode)
	}
	return h.tb.Send(c.Recipient(), text, htmlMode)
}
