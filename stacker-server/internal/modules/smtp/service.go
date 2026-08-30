package smtp

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
)

var (
	ErrNotConfigured = errors.New("smtp is not enabled or configured")
	ErrInvalidConfig = errors.New("enter a valid smtp host, port and from address")
)

type Service struct {
	repo *Repository
	key  []byte
	log  *slog.Logger
}

func NewService(repo *Repository, keyDir string, log *slog.Logger) (*Service, error) {
	key, err := loadOrCreateKey(keyDir)
	if err != nil {
		return nil, err
	}
	return &Service{repo: repo, key: key, log: log}, nil
}

func (s *Service) Enabled() bool {
	item, err := s.repo.Get()
	if err != nil {
		return false
	}
	return item.Enabled && item.Host != "" && item.FromEmail != ""
}

func (s *Service) Get() (SettingsResponse, error) {
	item, err := s.repo.Get()
	if err != nil {
		return SettingsResponse{}, err
	}
	return item.toResponse(), nil
}

func (s *Service) Update(req UpdateRequest) (SettingsResponse, error) {
	item, err := s.repo.Get()
	if err != nil {
		return SettingsResponse{}, err
	}

	item.Enabled = req.Enabled
	item.Host = strings.TrimSpace(req.Host)
	item.Port = req.Port
	item.Encryption = strings.TrimSpace(req.Encryption)
	item.Username = strings.TrimSpace(req.Username)
	item.FromName = strings.TrimSpace(req.FromName)
	item.FromEmail = strings.ToLower(strings.TrimSpace(req.FromEmail))

	if req.Password != "" {
		encrypted, err := encrypt(s.key, req.Password)
		if err != nil {
			return SettingsResponse{}, err
		}
		item.Password = encrypted
	}

	if item.Port <= 0 {
		item.Port = 587
	}
	if item.Encryption == "" {
		item.Encryption = "starttls"
	}

	if err := s.repo.Save(&item); err != nil {
		return SettingsResponse{}, err
	}
	return item.toResponse(), nil
}

func (s *Service) SendTest(to string) error {
	return s.Send(to, "Stacker test email", "This message confirms your SMTP settings are working.")
}

func (s *Service) Send(to, subject, body string) error {
	item, err := s.repo.Get()
	if err != nil {
		return err
	}
	if !item.Enabled {
		return ErrNotConfigured
	}
	if item.Host == "" || item.FromEmail == "" {
		return ErrInvalidConfig
	}

	password, err := decrypt(s.key, item.Password)
	if err != nil {
		return fmt.Errorf("smtp: decrypt password: %w", err)
	}
	item.Password = password

	from := formatAddress(item.FromName, item.FromEmail)
	msg := buildMessage(from, to, subject, body)
	addr := fmt.Sprintf("%s:%d", item.Host, item.Port)

	switch strings.ToLower(item.Encryption) {
	case "tls":
		return sendTLS(addr, item, from, []string{to}, msg)
	case "none":
		return smtp.SendMail(addr, plainAuth(item), item.FromEmail, []string{to}, msg)
	default:
		return sendSTARTTLS(addr, item, from, []string{to}, msg)
	}
}

func buildMessage(from, to, subject, body string) []byte {
	headers := []string{
		"From: " + from,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}
	return []byte(strings.Join(headers, "\r\n"))
}

func formatAddress(name, email string) string {
	if name == "" {
		return email
	}
	return fmt.Sprintf("%s <%s>", name, email)
}

func plainAuth(item Settings) smtp.Auth {
	if item.Username == "" {
		return nil
	}
	return smtp.PlainAuth("", item.Username, item.Password, item.Host)
}

func sendSTARTTLS(addr string, item Settings, from string, to []string, msg []byte) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close() //nolint:errcheck

	client, err := smtp.NewClient(conn, item.Host)
	if err != nil {
		return err
	}
	defer client.Close() //nolint:errcheck

	if err = client.Hello("stacker"); err != nil {
		return err
	}
	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsConfig := &tls.Config{ServerName: item.Host}
		if err = client.StartTLS(tlsConfig); err != nil {
			return err
		}
	}
	if auth := plainAuth(item); auth != nil {
		if err = client.Auth(auth); err != nil {
			return err
		}
	}
	return sendClient(client, item.FromEmail, to, msg)
}

func sendTLS(addr string, item Settings, from string, to []string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: item.Host})
	if err != nil {
		return err
	}
	defer conn.Close() //nolint:errcheck

	client, err := smtp.NewClient(conn, item.Host)
	if err != nil {
		return err
	}
	defer client.Close() //nolint:errcheck

	if err = client.Hello("stacker"); err != nil {
		return err
	}
	if auth := plainAuth(item); auth != nil {
		if err = client.Auth(auth); err != nil {
			return err
		}
	}
	return sendClient(client, item.FromEmail, to, msg)
}

func sendClient(client *smtp.Client, from string, to []string, msg []byte) error {
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err = writer.Write(msg); err != nil {
		_ = writer.Close()
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}
