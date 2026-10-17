package notify

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestSendPasswordResetEmail_CancelsStalledServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		close(accepted)
		_, _ = bufio.NewReader(conn).ReadString('\n')
	}()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	sender, err := NewSMTPSender(SMTPConfig{Host: host, Port: port, From: "support@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sendDone := make(chan error, 1)
	go func() {
		sendDone <- sender.SendPasswordResetEmail(ctx, "customer@example.com", "https://shop.example.com/reset")
	}()
	select {
	case <-accepted:
	case <-time.After(3 * time.Second):
		t.Fatal("no connection")
	}
	cancel()
	select {
	case err := <-sendDone:
		if err == nil {
			t.Fatal("cancelled send succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("SMTP ignored cancellation")
	}
	<-done
}

func TestSendPasswordResetEmail_RequiresProductionTLS(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		_, _ = fmt.Fprint(conn, "220 localhost ESMTP\r\n")
		_, _ = bufio.NewReader(conn).ReadString('\n')
		_, _ = fmt.Fprint(conn, "250 localhost\r\n")
	}()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	sender, err := NewSMTPSender(SMTPConfig{Host: host, Port: port, From: "support@example.com", RequireTLS: true})
	if err != nil {
		t.Fatal(err)
	}
	err = sender.SendPasswordResetEmail(context.Background(), "customer@example.com", "https://shop.example.com/reset")
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("error = %v, want required TLS", err)
	}
	<-done
}

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
