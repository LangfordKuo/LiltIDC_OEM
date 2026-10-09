package notify

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestBuildEmailMessage(t *testing.T) {
	raw := BuildEmailMessage("from@example.com", "to@example.com", "测试主题", "<p>你好 world</p>")
	msg := string(raw)

	for _, want := range []string{
		"From: from@example.com\r\n",
		"To: to@example.com\r\n",
		"MIME-Version: 1.0\r\n",
		"Content-Type: text/html; charset=UTF-8\r\n",
		"Content-Transfer-Encoding: base64\r\n",
		"\r\n\r\n", // 头与正文的分隔
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("邮件原文缺少 %q:\n%s", want, msg)
		}
	}
	// 中文主题必须做 RFC 2047 编码，不能出现裸中文
	if strings.Contains(msg, "Subject: 测试主题") {
		t.Error("Subject 未做 RFC 2047 编码")
	}
	if !strings.Contains(msg, "Subject: =?UTF-8?q?") {
		t.Errorf("Subject Q 编码缺失:\n%s", msg)
	}
	// 正文 base64 解码后应还原
	parts := strings.SplitN(msg, "\r\n\r\n", 2)
	if len(parts) != 2 {
		t.Fatalf("邮件头正文分隔失败")
	}
	body := strings.ReplaceAll(strings.TrimSpace(parts[1]), "\r\n", "")
	decoded, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		t.Fatalf("正文 base64 解码失败: %v", err)
	}
	if string(decoded) != "<p>你好 world</p>" {
		t.Errorf("正文还原错误: %s", decoded)
	}
	// base64 行长度不超过 76（RFC 2045）
	for _, line := range strings.Split(strings.TrimSpace(parts[1]), "\r\n") {
		if len(line) > 76 {
			t.Errorf("base64 行超长(%d): %s", len(line), line)
		}
	}
}

// fakeSMTPServer 是最小可用的明文 SMTP 服务器（测试用，不支持 TLS）。
// 返回监听地址与接收 DATA 内容的通道。
func fakeSMTPServer(t *testing.T) (string, chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	dataCh := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		r := bufio.NewReader(conn)
		write := func(s string) { _, _ = fmt.Fprint(conn, s) }

		write("220 fake ESMTP\r\n")
		inData := false
		var dataBuf strings.Builder
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			trimmed := strings.TrimRight(line, "\r\n")
			if inData {
				if trimmed == "." {
					dataCh <- dataBuf.String()
					write("250 OK queued\r\n")
					inData = false
					continue
				}
				dataBuf.WriteString(line)
				continue
			}
			upper := strings.ToUpper(trimmed)
			switch {
			case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
				write("250-fake\r\n250 AUTH PLAIN LOGIN\r\n")
			case strings.HasPrefix(upper, "AUTH"):
				write("235 Authentication successful\r\n")
			case strings.HasPrefix(upper, "MAIL FROM"):
				write("250 OK\r\n")
			case strings.HasPrefix(upper, "RCPT TO"):
				write("250 OK\r\n")
			case strings.HasPrefix(upper, "DATA"):
				write("354 End data with <CR><LF>.<CR><LF>\r\n")
				inData = true
			case strings.HasPrefix(upper, "QUIT"):
				write("221 Bye\r\n")
				return
			default:
				write("250 OK\r\n")
			}
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return ln.Addr().String(), dataCh
}

func TestSMTPSendFlow(t *testing.T) {
	addr, dataCh := fakeSMTPServer(t)
	host, portStr, _ := net.SplitHostPort(addr)
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		t.Fatalf("解析端口失败: %v", err)
	}

	s := NewSMTP(SMTPConfig{
		Host:     host,
		Port:     port,
		Username: "user@example.com",
		Password: "pwd",
		From:     "user@example.com",
		TLS:      false, // 假服务器不支持 STARTTLS
	})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	err := s.Send(ctx, &Message{To: "dest@example.com", Subject: "Hello", Content: "<b>hi</b>"})
	if err != nil {
		t.Fatalf("SMTP Send 失败: %v", err)
	}
	select {
	case data := <-dataCh:
		if !strings.Contains(data, "To: dest@example.com") {
			t.Errorf("服务器收到的邮件缺少收件人头:\n%s", data)
		}
		if !strings.Contains(data, "Content-Type: text/html") {
			t.Errorf("服务器收到的邮件缺少 HTML 内容类型:\n%s", data)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("超时：SMTP 服务器未收到 DATA 内容")
	}
}

func TestSMTPSendNotConfigured(t *testing.T) {
	s := NewSMTP(SMTPConfig{})
	err := s.Send(context.Background(), &Message{To: "a@b.c"})
	if err != ErrNotConfigured {
		t.Errorf("空配置应返回 ErrNotConfigured，得到: %v", err)
	}
}

func TestSMTPSendTLSServerUnsupported(t *testing.T) {
	addr, _ := fakeSMTPServer(t)
	host, portStr, _ := net.SplitHostPort(addr)
	var port int
	_, _ = fmt.Sscanf(portStr, "%d", &port)

	s := NewSMTP(SMTPConfig{Host: host, Port: port, From: "a@b.c", TLS: true})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	err := s.Send(ctx, &Message{To: "d@e.f", Subject: "s", Content: "c"})
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Errorf("要求 TLS 但服务器不支持时应报 STARTTLS 错误，得到: %v", err)
	}
}

func TestSMTPSendConnectRefused(t *testing.T) {
	// 127.0.0.1:1 基本必然拒绝连接
	s := NewSMTP(SMTPConfig{Host: "127.0.0.1", Port: 1, From: "a@b.c"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := s.Send(ctx, &Message{To: "d@e.f", Subject: "s", Content: "c"})
	if err == nil || !strings.Contains(err.Error(), "连接 SMTP 服务器") {
		t.Errorf("连接拒绝应报可读错误，得到: %v", err)
	}
}
