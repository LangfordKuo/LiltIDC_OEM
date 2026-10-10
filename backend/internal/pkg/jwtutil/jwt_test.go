package jwtutil

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "unit-test-secret-0123456789"

func TestSignParseRoundTrip(t *testing.T) {
	token, expiresAt, err := Sign(testSecret, time.Hour, 42, "admin")
	if err != nil {
		t.Fatalf("Sign 失败: %v", err)
	}
	if token == "" {
		t.Fatal("token 为空")
	}
	if time.Until(expiresAt) <= 0 || time.Until(expiresAt) > time.Hour {
		t.Errorf("过期时间异常: %v", expiresAt)
	}
	claims, err := Parse(testSecret, token)
	if err != nil {
		t.Fatalf("Parse 失败: %v", err)
	}
	if claims.AdminID != 42 || claims.Username != "admin" {
		t.Errorf("claims 不一致: %+v", claims)
	}
	if claims.Issuer != Issuer {
		t.Errorf("issuer 不一致: %s", claims.Issuer)
	}
}

func TestParseWrongSecret(t *testing.T) {
	token, _, err := Sign(testSecret, time.Hour, 1, "admin")
	if err != nil {
		t.Fatalf("Sign 失败: %v", err)
	}
	if _, err := Parse("another-secret", token); err == nil {
		t.Fatal("错误 secret 应校验失败")
	}
}

func TestParseExpired(t *testing.T) {
	token, _, err := Sign(testSecret, -time.Minute, 1, "admin")
	if err != nil {
		t.Fatalf("Sign 失败: %v", err)
	}
	_, err = Parse(testSecret, token)
	if err != ErrExpired {
		t.Fatalf("过期 token 应返回 ErrExpired，得到 %v", err)
	}
}

func TestParseGarbage(t *testing.T) {
	if _, err := Parse(testSecret, "not-a-jwt"); err != ErrInvalid {
		t.Fatalf("垃圾 token 应返回 ErrInvalid，得到 %v", err)
	}
	if _, err := Parse(testSecret, ""); err != ErrInvalid {
		t.Fatalf("空 token 应返回 ErrInvalid，得到 %v", err)
	}
}

func TestParseAlgConfusion(t *testing.T) {
	// 用 none 算法构造的 token 必须被拒绝（防算法混淆攻击）
	token := jwt.NewWithClaims(jwt.SigningMethodNone, &Claims{AdminID: 1, Username: "hacker"})
	s, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		return // 构造阶段就失败，同样视为通过
	}
	if _, err := Parse(testSecret, s); err == nil {
		t.Fatal("none 算法 token 不应通过校验")
	}
}

func TestSignEmptySecret(t *testing.T) {
	if _, _, err := Sign("", time.Hour, 1, "admin"); err == nil {
		t.Fatal("空 secret 应拒绝签发")
	}
}

// ---- 批 3-α：会员 token 与管理员 token 双签发方隔离 ----

func TestSignParseClientRoundTrip(t *testing.T) {
	token, expiresAt, err := SignClient(testSecret, time.Hour, 7, "member01")
	if err != nil {
		t.Fatalf("SignClient 失败: %v", err)
	}
	if time.Until(expiresAt) <= 0 || time.Until(expiresAt) > time.Hour {
		t.Errorf("过期时间异常: %v", expiresAt)
	}
	claims, err := ParseClient(testSecret, token)
	if err != nil {
		t.Fatalf("ParseClient 失败: %v", err)
	}
	if claims.ClientID != 7 || claims.Username != "member01" {
		t.Errorf("claims 不一致: %+v", claims)
	}
	if claims.Issuer != IssuerClient {
		t.Errorf("issuer 应为 %s，得到 %s", IssuerClient, claims.Issuer)
	}
}

func TestClientTokenRejectedByAdminParse(t *testing.T) {
	// 会员 token 拿去访问管理员接口必须 401（issuer 不匹配）
	token, _, err := SignClient(testSecret, time.Hour, 7, "member01")
	if err != nil {
		t.Fatalf("SignClient 失败: %v", err)
	}
	if _, err := Parse(testSecret, token); err == nil {
		t.Fatal("管理员 Parse 不应接受会员 token")
	}
}

func TestAdminTokenRejectedByClientParse(t *testing.T) {
	// 管理员 token 拿去访问会员接口必须 401（防越权）
	token, _, err := Sign(testSecret, time.Hour, 1, "admin")
	if err != nil {
		t.Fatalf("Sign 失败: %v", err)
	}
	if _, err := ParseClient(testSecret, token); err == nil {
		t.Fatal("会员 ParseClient 不应接受管理员 token")
	}
}

func TestParseClientExpired(t *testing.T) {
	token, _, err := SignClient(testSecret, -time.Minute, 7, "member01")
	if err != nil {
		t.Fatalf("SignClient 失败: %v", err)
	}
	if _, err := ParseClient(testSecret, token); err != ErrExpired {
		t.Fatalf("过期会员 token 应返回 ErrExpired，得到 %v", err)
	}
}
