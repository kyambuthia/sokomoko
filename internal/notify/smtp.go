package notify

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

const defaultSMTPPort = "587"

type SMTPConfig struct {
	Host       string
	Port       string
	Username   string
	Password   string
	From       string
	AppName    string
	RequireTLS bool
}

type SMTPSender struct {
	cfg SMTPConfig
}

func NewSMTPSender(cfg SMTPConfig) (*SMTPSender, error) {
	cfg.Host = strings.TrimSpace(strings.ToLower(cfg.Host))
	cfg.Port = strings.TrimSpace(cfg.Port)
	cfg.Username = strings.TrimSpace(cfg.Username)
	cfg.Password = strings.TrimSpace(cfg.Password)
	cfg.From = strings.TrimSpace(cfg.From)
	cfg.AppName = strings.TrimSpace(cfg.AppName)

	if cfg.Port == "" {
		cfg.Port = defaultSMTPPort
	}
	if cfg.AppName == "" {
		cfg.AppName = "Sokomoko"
	}
	if cfg.Host == "" {
		return nil, fmt.Errorf("smtp host is required")
	}
	if cfg.From == "" {
		return nil, fmt.Errorf("smtp from address is required")
	}
	if _, err := mail.ParseAddress(cfg.From); err != nil || strings.ContainsAny(cfg.From, "\r\n") {
		return nil, fmt.Errorf("invalid smtp from address: %w", err)
	}

	return &SMTPSender{cfg: cfg}, nil
}

func (s *SMTPSender) SendPasswordResetEmail(ctx context.Context, recipientEmail string, resetLink string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	recipient := strings.TrimSpace(recipientEmail)
	if recipient == "" {
		return fmt.Errorf("recipient email is required")
	}
	to, err := mail.ParseAddress(recipient)
	if err != nil || strings.ContainsAny(recipient, "\r\n") {
		return fmt.Errorf("invalid recipient email: %w", err)
	}
	if strings.TrimSpace(resetLink) == "" {
		return fmt.Errorf("reset link is required")
	}

	message := buildPasswordResetMessage(s.cfg.From, recipient, s.cfg.AppName, resetLink)
	address := net.JoinHostPort(s.cfg.Host, s.cfg.Port)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	} else if s.cfg.RequireTLS {
		return fmt.Errorf("SMTP server must support STARTTLS")
	}
	if s.cfg.Username != "" && s.cfg.Password != "" {
		if err := client.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return err
		}
	}
	from, _ := mail.ParseAddress(s.cfg.From) // validated by NewSMTPSender
	if err := client.Mail(from.Address); err != nil {
		return err
	}
	if err := client.Rcpt(to.Address); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write([]byte(message)); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func buildPasswordResetMessage(from string, to string, appName string, resetLink string) string {
	subject := appName + " password reset"
	body := "Hello,\r\n\r\n" +
		"We received a password reset request for your " + appName + " account.\r\n" +
		"Use this link to choose a new password:\r\n" +
		resetLink + "\r\n\r\n" +
		"If you did not request this reset, you can ignore this email.\r\n"

	return "From: " + from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" +
		body
}
