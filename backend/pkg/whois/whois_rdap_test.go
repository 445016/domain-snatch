package whois

import (
	"testing"
)

func TestQueryWithIdentityDigitalRDAP_jupiter_money(t *testing.T) {
	result, err := QueryWithIdentityDigitalRDAP("jupiter.money")
	if err != nil {
		t.Fatalf("QueryWithIdentityDigitalRDAP: %v", err)
	}
	if result.Domain != "jupiter.money" {
		t.Errorf("Domain = %q, want jupiter.money", result.Domain)
	}
	if result.Status != StatusRegistered {
		t.Errorf("Status = %q, want %q", result.Status, StatusRegistered)
	}
	if result.ExpiryDate == nil {
		t.Error("ExpiryDate is nil")
	}
	if result.Registrar == "" {
		t.Error("Registrar is empty")
	}
	if result.WhoisRaw == "" {
		t.Error("WhoisRaw is empty")
	}
	t.Logf("OK: status=%s, registrar=%s, expiry=%v", result.Status, result.Registrar, result.ExpiryDate)
}

func TestQueryRDAPBootstrap_bachatt_app(t *testing.T) {
	result, err := QueryRDAPBootstrap("bachatt.app")
	if err != nil {
		t.Fatalf("QueryRDAPBootstrap: %v", err)
	}
	if result.Domain != "bachatt.app" {
		t.Errorf("Domain = %q, want bachatt.app", result.Domain)
	}
	if result.Status != StatusRegistered {
		t.Errorf("Status = %q, want %q", result.Status, StatusRegistered)
	}
	if result.ExpiryDate == nil {
		t.Error("ExpiryDate is nil")
	}
	if result.Registrar == "" {
		t.Error("Registrar is empty")
	}
	if result.WhoisRaw == "" {
		t.Error("WhoisRaw is empty")
	}
	t.Logf("OK: status=%s, registrar=%s, expiry=%v", result.Status, result.Registrar, result.ExpiryDate)
}

func TestQuery_swanfinance_in(t *testing.T) {
	// .in 直连 NIXI，应得到 status=restricted（restricted by registry policy）；cron 可通过状态过滤
	result, err := Query("swanfinance.in")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if result.Domain != "swanfinance.in" {
		t.Errorf("Domain = %q, want swanfinance.in", result.Domain)
	}
	if result.Status != StatusRestricted {
		t.Errorf("Status = %q, want %q", result.Status, StatusRestricted)
	}
	if result.CanRegister {
		t.Error("restricted .in should have CanRegister=false")
	}
	t.Logf("OK: status=%s, hasExpiry=%v", result.Status, result.ExpiryDate != nil)
}

func TestQuery_onescore_app(t *testing.T) {
	// .app 走 RDAP 优先，应得到正确域名信息而非 IANA TLD 解析结果
	result, err := Query("onescore.app")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if result.Domain != "onescore.app" {
		t.Errorf("Domain = %q, want onescore.app", result.Domain)
	}
	if result.Status != StatusRegistered && result.Status != StatusExpired {
		t.Errorf("Status = %q, expect registered or expired", result.Status)
	}
	if result.Status == StatusRegistered && result.ExpiryDate == nil {
		t.Error("registered .app should have ExpiryDate from RDAP")
	}
	t.Logf("OK: status=%s, registrar=%s, expiry=%v", result.Status, result.Registrar, result.ExpiryDate)
}

func TestQuery_jindal_bz(t *testing.T) {
	// .bz 默认 whois 只返回 IANA，应走 QueryWithAfiliasServer 得到正确解析
	result, err := Query("jindal.bz")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if result.Domain != "jindal.bz" {
		t.Errorf("Domain = %q, want jindal.bz", result.Domain)
	}
	if result.Status != StatusRegistered && result.Status != StatusExpired {
		t.Errorf("Status = %q, expect registered or expired", result.Status)
	}
	if result.ExpiryDate == nil {
		t.Error("ExpiryDate should be set from Afilias whois")
	}
	if result.Registrar == "" {
		t.Error("Registrar should be set")
	}
	t.Logf("OK: status=%s, registrar=%s, expiry=%v", result.Status, result.Registrar, result.ExpiryDate)
}
