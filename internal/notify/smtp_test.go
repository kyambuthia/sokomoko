package notify

import (
	"context"
	"strings"
	"testing"
)

func TestNewSMTPSender_DefaultsAndValidation(t *testing.T) {
	if _, err := NewSMTPSender(SMTPConfig{From: "alerts@example.com"}); err == nil {
		t.Fatal("expected missing host error")
	}
	if _, err := NewSMTPSender(SMTPConfig{Host: "smtp.example.com", From: "invalid"}); err == nil {
		t.Fatal("expected invalid from email error")
	}

	sender, err := NewSMTPSender(SMTPConfig{
		Host: "SMTP.EXAMPLE.COM",
		From: "alerts@example.com",
	})
	if err != nil {
		t.Fatalf("new smtp sender: %v", err)
	}
	if sender.cfg.Host != "smtp.example.com" {
		t.Fatalf("host=%q want=smtp.example.com", sender.cfg.Host)
	}
	if sender.cfg.Port != defaultSMTPPort {
		t.Fatalf("port=%q want=%q", sender.cfg.Port, defaultSMTPPort)
	}
	if sender.cfg.AppName != "Sokomoko" {
		t.Fatalf("app name=%q want=Sokomoko", sender.cfg.AppName)
	}
}

func TestBuildPasswordResetMessage(t *testing.T) {
	msg := buildPasswordResetMessage("alerts@example.com", "person@example.com", "Sokomoko", "https://shop.example.com/password-reset/confirm?token=abc")

	if !strings.Contains(msg, "Subject: Sokomoko password reset") {
		t.Fatal("expected subject header in message")
	}
	if !strings.Contains(msg, "https://shop.example.com/password-reset/confirm?token=abc") {
		t.Fatal("expected reset link in message body")
	}
	if !strings.Contains(msg, "From: alerts@example.com") || !strings.Contains(msg, "To: person@example.com") {
		t.Fatal("expected from/to headers in message")
	}
}

func TestSendPasswordResetEmail_RejectsInvalidRecipient(t *testing.T) {
	sender, err := NewSMTPSender(SMTPConfig{
		Host: "smtp.example.com",
		From: "alerts@example.com",
	})
	if err != nil {
		t.Fatalf("new smtp sender: %v", err)
	}

	err = sender.SendPasswordResetEmail(context.Background(), "bad-email", "https://shop.example.com/password-reset/confirm?token=abc")
	if err == nil {
		t.Fatal("expected invalid recipient error")
	}
}
