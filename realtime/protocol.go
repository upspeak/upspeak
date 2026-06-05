package realtime

import "errors"

// applyClientMessage processes one decoded control message against the
// connection, mutating its subscription set. It returns a non-nil errorMessage
// to send back to the client on failure, or nil on success.
func applyClientMessage(c *connection, msg clientMessage, r refResolver) *errorMessage {
	switch msg.Action {
	case "subscribe":
		return subscribe(c, msg, r)
	case "unsubscribe":
		c.removeSubscription(msg.Channel)
		return nil
	default:
		return &errorMessage{
			Type:    "error",
			Code:    codeInvalidChannel,
			Message: "unknown action: " + msg.Action,
		}
	}
}

// subscribe parses and resolves the channel, then registers the subscription,
// mapping each failure to the appropriate client error code.
func subscribe(c *connection, msg clientMessage, r refResolver) *errorMessage {
	pc, err := parseChannel(msg.Channel)
	if err != nil {
		return &errorMessage{Type: "error", Code: codeInvalidChannel, Message: err.Error()}
	}
	sub, err := resolveChannel(pc, msg.Filter, r)
	if err != nil {
		return &errorMessage{Type: "error", Code: codeInvalidChannel, Message: err.Error()}
	}
	if err := c.addSubscription(sub); err != nil {
		if errors.Is(err, errSubscriptionLimit) {
			return &errorMessage{Type: "error", Code: codeSubscriptionLimit, Message: err.Error()}
		}
		return &errorMessage{Type: "error", Code: codeInvalidChannel, Message: err.Error()}
	}
	return nil
}
