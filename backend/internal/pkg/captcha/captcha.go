// Package captcha 提供图形验证码生成（纯 Go 标准库 image/png，无第三方依赖）。
//
// 4 位字符（字符集去除 0/O/1/I 等易混淆字符），点阵字体放大渲染 +
// 随机偏移 + 干扰线 + 噪点。定位为**辅助防线**（防脚本批量轰验证码短信），
// 真正的频控在服务端（settings 计数，见 service/verifycode.go）。
package captcha

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	mrand "math/rand/v2"
)

// charset 去除了 0/O/1/I/L 等易混淆字符。
const charset = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

// font5x7 是字符集的点阵字模（每字符 7 行 × 5 列）。
var font5x7 = map[byte][7]string{
	'2': {"01110", "10001", "00001", "00010", "00100", "01000", "11111"},
	'3': {"01110", "10001", "00001", "00110", "00001", "10001", "01110"},
	'4': {"00010", "00110", "01010", "10010", "11111", "00010", "00010"},
	'5': {"11111", "10000", "11110", "00001", "00001", "10001", "01110"},
	'6': {"00110", "01000", "10000", "11110", "10001", "10001", "01110"},
	'7': {"11111", "00001", "00010", "00100", "01000", "01000", "01000"},
	'8': {"01110", "10001", "10001", "01110", "10001", "10001", "01110"},
	'9': {"01110", "10001", "10001", "01111", "00001", "00010", "01100"},
	'A': {"01110", "10001", "10001", "11111", "10001", "10001", "10001"},
	'B': {"11110", "10001", "10001", "11110", "10001", "10001", "11110"},
	'C': {"01110", "10001", "10000", "10000", "10000", "10001", "01110"},
	'D': {"11110", "10001", "10001", "10001", "10001", "10001", "11110"},
	'E': {"11111", "10000", "10000", "11110", "10000", "10000", "11111"},
	'F': {"11111", "10000", "10000", "11110", "10000", "10000", "10000"},
	'G': {"01110", "10001", "10000", "10111", "10001", "10001", "01111"},
	'H': {"10001", "10001", "10001", "11111", "10001", "10001", "10001"},
	'J': {"00111", "00010", "00010", "00010", "00010", "10010", "01100"},
	'K': {"10001", "10010", "10100", "11000", "10100", "10010", "10001"},
	'M': {"10001", "11011", "10101", "10101", "10001", "10001", "10001"},
	'N': {"10001", "11001", "10101", "10011", "10001", "10001", "10001"},
	'P': {"11110", "10001", "10001", "11110", "10000", "10000", "10000"},
	'Q': {"01110", "10001", "10001", "10001", "10101", "10010", "01101"},
	'R': {"11110", "10001", "10001", "11110", "10100", "10010", "10001"},
	'S': {"01111", "10000", "10000", "01110", "00001", "00001", "11110"},
	'T': {"11111", "00100", "00100", "00100", "00100", "00100", "00100"},
	'U': {"10001", "10001", "10001", "10001", "10001", "10001", "01110"},
	'V': {"10001", "10001", "10001", "10001", "10001", "01010", "00100"},
	'W': {"10001", "10001", "10001", "10101", "10101", "11011", "10001"},
	'X': {"10001", "10001", "01010", "00100", "01010", "10001", "10001"},
	'Y': {"10001", "10001", "01010", "00100", "00100", "00100", "00100"},
	'Z': {"11111", "00001", "00010", "00100", "01000", "10000", "11111"},
}

const (
	imgWidth  = 120
	imgHeight = 40
	charScale = 4 // 点阵放大倍数：5*4=20 宽、7*4=28 高
)

// Generate 生成一个图形验证码，返回答案明文与 PNG data URI。
// 答案比对由调用方负责（哈希存储，校验即焚，见 service 层）。
func Generate() (answer string, dataURI string, err error) {
	answer, err = randomAnswer(4)
	if err != nil {
		return "", "", err
	}

	img := image.NewRGBA(image.Rect(0, 0, imgWidth, imgHeight))
	// 浅色底 + 噪点
	bg := color.RGBA{R: uint8(235 + mrand.IntN(16)), G: uint8(235 + mrand.IntN(16)), B: uint8(235 + mrand.IntN(16)), A: 255}
	for y := 0; y < imgHeight; y++ {
		for x := 0; x < imgWidth; x++ {
			c := bg
			if mrand.IntN(12) == 0 { // 噪点
				c = color.RGBA{R: uint8(180 + mrand.IntN(50)), G: uint8(180 + mrand.IntN(50)), B: uint8(180 + mrand.IntN(50)), A: 255}
			}
			img.Set(x, y, c)
		}
	}
	// 干扰线（3 条随机深色直线）
	for i := 0; i < 3; i++ {
		lineColor := randDarkColor()
		x1, y1 := mrand.IntN(imgWidth), mrand.IntN(imgHeight)
		x2, y2 := mrand.IntN(imgWidth), mrand.IntN(imgHeight)
		drawLine(img, x1, y1, x2, y2, lineColor)
	}
	// 字符：随机深色 + 随机纵向偏移
	for i := 0; i < len(answer); i++ {
		glyph, ok := font5x7[answer[i]]
		if !ok {
			continue
		}
		fg := randDarkColor()
		ox := 6 + i*27 + mrand.IntN(4)
		oy := 4 + mrand.IntN(6)
		for row := 0; row < 7; row++ {
			for col := 0; col < 5; col++ {
				if glyph[row][col] != '1' {
					continue
				}
				for dy := 0; dy < charScale; dy++ {
					for dx := 0; dx < charScale; dx++ {
						img.Set(ox+col*charScale+dx, oy+row*charScale+dy, fg)
					}
				}
			}
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", "", fmt.Errorf("PNG 编码失败: %w", err)
	}
	return answer, "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// randomAnswer 用加密随机源生成 n 位答案。
func randomAnswer(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成验证码随机数失败: %w", err)
	}
	out := make([]byte, n)
	for i, b := range buf {
		out[i] = charset[int(b)%len(charset)]
	}
	return string(out), nil
}

// randDarkColor 生成随机深色（保证浅底可读）。
func randDarkColor() color.RGBA {
	return color.RGBA{R: uint8(mrand.IntN(120)), G: uint8(mrand.IntN(120)), B: uint8(mrand.IntN(120)), A: 255}
}

// drawLine 画直线（Bresenham）。
func drawLine(img *image.RGBA, x1, y1, x2, y2 int, c color.RGBA) {
	dx, dy := abs(x2-x1), abs(y2-y1)
	sx, sy := step(x1, x2), step(y1, y2)
	err := dx - dy
	for {
		if x1 >= 0 && x1 < imgWidth && y1 >= 0 && y1 < imgHeight {
			img.Set(x1, y1, c)
		}
		if x1 == x2 && y1 == y2 {
			return
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x1 += sx
		}
		if e2 < dx {
			err += dx
			y1 += sy
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func step(from, to int) int {
	if from < to {
		return 1
	}
	if from > to {
		return -1
	}
	return 0
}
