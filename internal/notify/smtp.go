package notify

import (
	"context"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
)

const defaultSMTPPort = "587"

type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
	AppName  string
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
	if _, err := mail.ParseAddress(cfg.From); err != nil {
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
	if _, err := mail.ParseAddress(recipient); err != nil {
		return fmt.Errorf("invalid recipient email: %w", err)
	}
	if strings.TrimSpace(resetLink) == "" {
		return fmt.Errorf("reset link is required")
	}

	message := buildPasswordResetMessage(s.cfg.From, recipient, s.cfg.AppName, resetLink)
	address := net.JoinHostPort(s.cfg.Host, s.cfg.Port)

	var auth smtp.Auth
	if s.cfg.Username != "" && s.cfg.Password != "" {
		auth = smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	}

	return smtp.SendMail(address, auth, s.cfg.From, []string{recipient}, []byte(message))
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
