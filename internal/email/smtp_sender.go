package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
)

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	TLSMode  string
}

type SMTPSender struct {
	config SMTPConfig
}

func NewSMTPSender(config SMTPConfig) *SMTPSender {
	return &SMTPSender{config: config}
}

func (s *SMTPSender) Send(ctx context.Context, message Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	to := strings.TrimSpace(message.To)
	if to == "" {
		return fmt.Errorf("email recipient is required")
	}
	from := strings.TrimSpace(s.config.From)
	if from == "" {
		return fmt.Errorf("email sender is required")
	}
	addr := net.JoinHostPort(s.config.Host, fmt.Sprintf("%d", s.config.Port))
	body := []byte("To: " + to + "\r\n" +
		"From: " + from + "\r\n" +
		"Subject: " + message.Subject + "\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" +
		message.TextBody + "\r\n")

	var auth smtp.Auth
	if s.config.Username != "" || s.config.Password != "" {
		auth = smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.Host)
	}
	if strings.EqualFold(s.config.TLSMode, "implicit") {
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: s.config.Host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return err
		}
		defer conn.Close()
		client, err := smtp.NewClient(conn, s.config.Host)
		if err != nil {
			return err
		}
		defer client.Close()
		return sendWithClient(client, auth, from, to, body)
	}
	if strings.EqualFold(s.config.TLSMode, "starttls") {
		client, err := smtp.Dial(addr)
		if err != nil {
			return err
		}
		defer client.Close()
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: s.config.Host, MinVersion: tls.VersionTLS12}); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("smtp server does not support STARTTLS")
		}
		return sendWithClient(client, auth, from, to, body)
	}
	return smtp.SendMail(addr, auth, from, []string{to}, body)
}

func sendWithClient(client *smtp.Client, auth smtp.Auth, from string, to string, body []byte) error {
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(body); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}
