package money

import "testing"

func TestParseCents(t *testing.T) {
	ok := []struct {
		in   string
		want int64
	}{
		{"0", 0}, {"0.00", 0}, {"12", 1200}, {"12.3", 1230}, {"12.34", 1234},
		{"+12.34", 1234}, {"-12.34", -1234}, {"-0.01", -1}, {" 8.88 ", 888},
		{"9999999999.99", maxCents}, {"007", 700},
	}
	for _, tc := range ok {
		got, err := ParseCents(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("ParseCents(%q) = %d, %v；期望 %d, nil", tc.in, got, err, tc.want)
		}
	}
	bad := []string{
		"", "abc", "12.345", "1.2.3", ".5", "5.", "--1", "++1", "-", "+",
		"1e3", "0x10", "12,34", "10000000000.00", // 超 DECIMAL(12,2)
		"９", // 全角数字
	}
	for _, in := range bad {
		if got, err := ParseCents(in); err == nil {
			t.Errorf("ParseCents(%q) = %d，期望报错", in, got)
		}
	}
}

func TestFormatCents(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0.00"}, {1234, "12.34"}, {-520, "-5.20"}, {5, "0.05"},
		{-1, "-0.01"}, {100, "1.00"}, {maxCents, "9999999999.99"},
	}
	for _, tc := range cases {
		if got := FormatCents(tc.in); got != tc.want {
			t.Errorf("FormatCents(%d) = %q；期望 %q", tc.in, got, tc.want)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	// 解析→格式化→再解析必须稳定（存库/读库往返不漂移）
	for _, s := range []string{"0.00", "20.00", "8.88", "1234567.89"} {
		c, err := ParseCents(s)
		if err != nil {
			t.Fatalf("ParseCents(%q): %v", s, err)
		}
		out := FormatCents(c)
		c2, err := ParseCents(out)
		if err != nil || c2 != c {
			t.Errorf("往返漂移: %q → %d → %q → %d", s, c, out, c2)
		}
	}
}

func TestNoFloatDrift(t *testing.T) {
	// 经典浮点陷阱：0.1+0.2 != 0.3（float64）；整数分运算必须精确
	a, _ := ParseCents("0.10")
	b, _ := ParseCents("0.20")
	if FormatCents(a+b) != "0.30" {
		t.Errorf("0.10+0.20 = %s，期望 0.30", FormatCents(a+b))
	}
	// 大额累加 1000 次不漂移
	sum := int64(0)
	v, _ := ParseCents("57664.50")
	for i := 0; i < 1000; i++ {
		sum += v
	}
	if sum != v*1000 {
		t.Errorf("累加漂移: %d != %d", sum, v*1000)
	}
}

func TestCmpAndIsValid(t *testing.T) {
	if Cmp(100, 200) != -1 || Cmp(200, 100) != 1 || Cmp(100, 100) != 0 {
		t.Error("Cmp 结果错误")
	}
	if !IsValid("12.34") || IsValid("12.345") || IsValid("") {
		t.Error("IsValid 结果错误")
	}
}
