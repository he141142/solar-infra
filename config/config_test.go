package config

import "testing"

func TestDevMayShareOneAccount(t *testing.T) {
	if _, err := Get("dev"); err != nil {
		t.Fatalf("dev should be valid without accounts: %v", err)
	}
}

func TestProductionNeedsOneAccountPerProject(t *testing.T) {
	e := Env{Name: "x", Production: true}
	if e.Validate() == nil {
		t.Error("missing accounts must be rejected")
	}
	e.Accounts = Accounts{EventBus: "111111111111", Trading: "111111111111", Forecast: "333333333333"}
	if e.Validate() == nil {
		t.Error("shared accounts must be rejected")
	}
	e.Accounts.Trading = "222222222222"
	if err := e.Validate(); err != nil {
		t.Errorf("distinct accounts should be valid: %v", err)
	}
}

func TestUnknownEnv(t *testing.T) {
	if _, err := Get("qa"); err == nil {
		t.Error("unknown env must be rejected")
	}
}
