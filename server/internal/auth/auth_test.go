package auth

import (
	"testing"
	"time"
)

func TestIssueParse(t *testing.T) {
	m := NewManager("secret", time.Hour)
	tok, err := m.Issue(42, 12345, "user")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	claims, err := m.Parse(tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.UserID != 42 || claims.TgUserID != 12345 || claims.Role != "user" {
		t.Fatalf("claims mismatch: %+v", claims)
	}
}

func TestParseWrongSecret(t *testing.T) {
	m := NewManager("secret", time.Hour)
	tok, _ := m.Issue(1, 2, "admin")
	other := NewManager("other", time.Hour)
	if _, err := other.Parse(tok); err == nil {
		t.Fatal("expected error for wrong secret")
	}
}

func TestParseGarbage(t *testing.T) {
	m := NewManager("secret", time.Hour)
	if _, err := m.Parse("not.a.jwt"); err == nil {
		t.Fatal("expected error for garbage")
	}
}

func TestExpired(t *testing.T) {
	m := NewManager("secret", -time.Minute)
	tok, err := m.Issue(1, 2, "user")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if _, err := m.Parse(tok); err == nil {
		t.Fatal("expected error for expired token")
	}
}