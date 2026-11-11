package config

import (
	"errors"
	"testing"
)

func TestFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    Config
		wantErr error
	}{
		{
			name: "defaults the listen address",
			env:  map[string]string{envDatabaseURL: "postgres://localhost/paddock"},
			want: Config{HTTPAddr: ":8080", DatabaseURL: "postgres://localhost/paddock"},
		},
		{
			name: "explicit listen address wins",
			env: map[string]string{
				envHTTPAddr:    "127.0.0.1:9000",
				envDatabaseURL: "postgres://localhost/paddock",
			},
			want: Config{HTTPAddr: "127.0.0.1:9000", DatabaseURL: "postgres://localhost/paddock"},
		},
		{
			name:    "database URL is required",
			env:     map[string]string{},
			wantErr: ErrMissing,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FromEnv(func(key string) string { return tt.env[key] })
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("config = %+v, want %+v", got, tt.want)
			}
		})
	}
}
