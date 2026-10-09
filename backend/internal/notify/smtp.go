package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"os"
	"strconv"
	"time"
)

// SMTPConfig 是 SMTP 邮件通道配置。
type SMTPConfig struct {
	Host     string
	Port     int    // 25/587 = 明文+STARTTLS；465 = 隐式 TLS（SMTPS）
	Username string // 留空则不做 AUTH
	Password string
	From     string // 发件人地址
	TLS      bool   // true = 要求 STARTTLS（465 端口隐式 TLS 时忽略本项）
}

// SMTP 是 net/smtp 实现的邮件 Sender。
type SMTP struct {
	cfg SMTPConfig
}

// NewSMTP 构造 SMTP provider。
func NewSMTP(cfg SMTPConfig) *SMTP {
	return &SMTP{cfg: cfg}
}

// Send 发送一封 HTML 邮件。
//
// 连接策略：
//   - 端口 465：隐式 TLS 直连（SMTPS）；
//   - 其它端口：明文连接，cfg.TLS=true 时强制 STARTTLS（服务器不支持则报错），
//     cfg.TLS=false 时保持明文。
func (s *SMTP) Send(ctx context.Context, msg *Message) error {
	if s.cfg.Host == "" || s.cfg.From == "" {
		return ErrNotConfigured
	}
	if msg.To == "" {
		return fmt.Errorf("收件人不能为空")
	}
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	raw := BuildEmailMessage(s.cfg.From, msg.To, msg.Subject, msg.Content)

	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	var err error
	implicitTLS := s.cfg.Port == 465
	if implicitTLS {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: s.cfg.Host})
		if err != nil {
			return fmt.Errorf("隐式 TLS 连接 SMTP 服务器 %s 失败: %w", addr, err)
		}
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("连接 SMTP 服务器 %s 失败: %w", addr, err)
		}
	}
	// 整体超时兜底：防止服务器挂起导致 goroutine 泄漏
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	}

	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("SMTP 握手失败（%s）: %w", addr, err)
	}
	defer client.Close()

	if err = client.Hello(localHostname()); err != nil {
		return fmt.Errorf("SMTP EHLO 失败: %w", err)
	}
	if !implicitTLS && s.cfg.TLS {
		ok, _ := client.Extension("STARTTLS")
		if !ok {
			return fmt.Errorf("SMTP 服务器 %s 不支持 STARTTLS，但配置要求 TLS", addr)
		}
		if err = client.StartTLS(&tls.Config{ServerName: s.cfg.Host}); err != nil {
			return fmt.Errorf("STARTTLS 握手失败（%s）: %w", addr, err)
		}
	}
	if s.cfg.Username != "" {
		ok, _ := client.Extension("AUTH")
		if !ok {
			return fmt.Errorf("SMTP 服务器 %s 不支持 AUTH 认证", addr)
		}
		if err = client.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return fmt.Errorf("SMTP 认证失败（用户名 %s）: %w", s.cfg.Username, err)
		}
	}
	if err = client.Mail(s.cfg.From); err != nil {
		return fmt.Errorf("MAIL FROM(%s) 被服务器拒绝: %w", s.cfg.From, err)
	}
	if err = client.Rcpt(msg.To); err != nil {
		return fmt.Errorf("收件人 %s 被服务器拒绝: %w", msg.To, err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA 被服务器拒绝: %w", err)
	}
	if _, err = w.Write(raw); err != nil {
		return fmt.Errorf("写入邮件内容失败: %w", err)
	}
	if err = w.Close(); err != nil {
		return fmt.Errorf("提交邮件内容失败: %w", err)
	}
	_ = client.Quit()
	return nil
}

// BuildEmailMessage 构造 RFC 822 格式的 HTML 邮件原文。
// 主题做 RFC 2047 Q 编码（支持中文）；正文 base64 编码、76 字符折行（RFC 2045）。
// 导出以便单元测试。
func BuildEmailMessage(from, to, subject, htmlBody string) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "From: %s\r\n", from)
	fmt.Fprintf(&buf, "To: %s\r\n", to)
	fmt.Fprintf(&buf, "Subject: %s\r\n", mime.QEncoding.Encode("UTF-8", subject))
	fmt.Fprintf(&buf, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	buf.WriteString("Content-Transfer-Encoding: base64\r\n")
	buf.WriteString("\r\n")
	encoded := base64.StdEncoding.EncodeToString([]byte(htmlBody))
	for i := 0; i < len(encoded); i += 76 {
		end := i + 76
		if end > len(encoded) {
			end = len(encoded)
		}
		buf.WriteString(encoded[i:end])
		buf.WriteString("\r\n")
	}
	return buf.Bytes()
}

// localHostname 返回 EHLO 用的本机名（失败时回退 localhost）。
func localHostname() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "localhost"
}
