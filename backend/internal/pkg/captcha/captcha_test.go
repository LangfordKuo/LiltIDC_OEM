package captcha

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	answer, dataURI, err := Generate()
	if err != nil {
		t.Fatalf("Generate 失败: %v", err)
	}
	if len(answer) != 4 {
		t.Errorf("答案应为 4 位，得到 %q", answer)
	}
	for i := 0; i < len(answer); i++ {
		if !strings.ContainsRune(charset, rune(answer[i])) {
			t.Errorf("答案含字符集外字符: %q", answer[i])
		}
		// 易混淆字符必须被排除
		if strings.ContainsRune("0O1IL", rune(answer[i])) {
			t.Errorf("答案含易混淆字符: %c", answer[i])
		}
	}
	if !strings.HasPrefix(dataURI, "data:image/png;base64,") {
		t.Fatalf("dataURI 前缀错误: %.40s", dataURI)
	}
	// base64 可解码且是合法 PNG（可被浏览器渲染）
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(dataURI, "data:image/png;base64,"))
	if err != nil {
		t.Fatalf("base64 解码失败: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("PNG 解码失败: %v", err)
	}
	if b := img.Bounds(); b.Dx() != imgWidth || b.Dy() != imgHeight {
		t.Errorf("图片尺寸错误: %v", b)
	}
}

func TestGenerateRandomness(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		answer, _, err := Generate()
		if err != nil {
			t.Fatalf("Generate 失败: %v", err)
		}
		if seen[answer] {
			// 20 次内出现重复的概率极低（31^4 空间），重复说明随机源有问题
			t.Errorf("答案重复: %q", answer)
		}
		seen[answer] = true
	}
}
