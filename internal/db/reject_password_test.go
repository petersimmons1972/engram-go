package db

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRejectDefaultPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{name: "empty password blocked", password: "", wantErr: true},
		{name: "engram default blocked", password: "engram", wantErr: true},
		{name: "postgres default blocked", password: "postgres", wantErr: true},
		{name: "env.example placeholder blocked", password: "change_me_to_a_strong_password", wantErr: true},
		{name: "strong password allowed", password: "str0ng-P@ssw0rd-2024!", wantErr: false},
	}

	// Parse a base config once; each sub-test overrides just the password field.
	baseDSN := "postgres://user:placeholder@localhost:5432/engram"
	baseCfg, err := pgxpool.ParseConfig(baseDSN)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Shallow-copy the pool config so tests don't interfere.
			cfg := *baseCfg
			connCfgCopy := *baseCfg.ConnConfig
			cfg.ConnConfig = &connCfgCopy
			cfg.ConnConfig.Password = tt.password

			err := rejectDefaultPassword(&cfg)
			if tt.wantErr && err == nil {
				t.Errorf("expected error for password %q, got nil", tt.password)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error for password %q, got: %v", tt.password, err)
			}
		})
	}
}
