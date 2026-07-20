// Package email sends transactional mail (verification links, OTP codes,
// set-password links). The dev default just logs; production uses SMTP via the
// well-maintained go-mail library.
package email

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/wneessen/go-mail"

	"github.com/thinkparq/edconsultancy-be/internal/config"
)

// Sender delivers an HTML email.
type Sender interface {
	Send(ctx context.Context, to, subject, htmlBody string) error
}

// New picks an implementation from config (EMAIL_SENDER = log | smtp).
func New(cfg *config.Config, logger *slog.Logger) Sender {
	if cfg.EmailSender == "smtp" && cfg.SMTPHost != "" {
		return &smtpSender{cfg: cfg, logger: logger}
	}
	return &logSender{logger: logger}
}

type logSender struct{ logger *slog.Logger }

func (l *logSender) Send(ctx context.Context, to, subject, htmlBody string) error {
	l.logger.InfoContext(ctx, "email (log sender)", "to", to, "subject", subject, "body", htmlBody)
	return nil
}

type smtpSender struct {
	cfg    *config.Config
	logger *slog.Logger
}

func (s *smtpSender) Send(ctx context.Context, to, subject, htmlBody string) error {
	msg := mail.NewMsg()
	if err := msg.FromFormat(s.cfg.EmailFromName, s.cfg.EmailFrom); err != nil {
		return fmt.Errorf("set from: %w", err)
	}
	if err := msg.To(to); err != nil {
		return fmt.Errorf("set to: %w", err)
	}
	msg.Subject(subject)
	msg.SetBodyString(mail.TypeTextHTML, htmlBody)

	opts := []mail.Option{mail.WithPort(s.cfg.SMTPPort)}
	if s.cfg.SMTPUsername != "" {
		opts = append(opts,
			mail.WithSMTPAuth(mail.SMTPAuthPlain),
			mail.WithUsername(s.cfg.SMTPUsername),
			mail.WithPassword(s.cfg.SMTPPassword),
		)
	}
	client, err := mail.NewClient(s.cfg.SMTPHost, opts...)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	return client.DialAndSendWithContext(ctx, msg)
}
