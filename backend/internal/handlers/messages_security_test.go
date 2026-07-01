package handlers

import (
	"strings"
	"testing"
)

func TestValidateOutboundMessageRejectsHeaderInjection(t *testing.T) {
	base := outboundMessage{
		FromAddress: "sender@example.com",
		ToAddresses: []string{"recipient@example.net"},
		Subject:     "Hello",
	}
	tests := []outboundMessage{
		func() outboundMessage {
			msg := base
			msg.Subject = "Hello\r\nBcc: attacker@example.org"
			return msg
		}(),
		func() outboundMessage {
			msg := base
			msg.ToAddresses = []string{"recipient@example.net\r\nBcc: attacker@example.org"}
			return msg
		}(),
		func() outboundMessage {
			msg := base
			msg.Attachments = []attachmentInput{{Filename: "safe.txt\r\nX-Evil: true"}}
			return msg
		}(),
	}
	for _, msg := range tests {
		if err := validateOutboundMessage(msg); err == nil {
			t.Fatal("header injection payload was accepted")
		}
	}
}

func TestBuildMIMEMessageEncodesBodySeparatelyFromHeaders(t *testing.T) {
	msg := outboundMessage{
		FromAddress: "sender@example.com",
		ToAddresses: []string{"recipient@example.net"},
		Subject:     "Hello",
		BodyHTML:    "<p>Body</p>\r\nBcc: not-a-header@example.org",
	}
	if err := validateOutboundMessage(msg); err != nil {
		t.Fatal(err)
	}
	raw, err := buildMIMEMessage(msg)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(string(raw), "\r\n\r\n", 2)
	if len(parts) != 2 {
		t.Fatal("message has no header/body separator")
	}
	if strings.Contains(parts[0], "not-a-header@example.org") {
		t.Fatal("body content escaped into MIME headers")
	}
}
