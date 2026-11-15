package model_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/viesloxy/ngawimusical-backend/app/model"
	"github.com/viesloxy/ngawimusical-backend/helper"
)

func TestRegisterRequestValidation(t *testing.T) {
	valid := model.RegisterRequest{Username: "arsalan", Email: "arsalan@example.com", Password: "rahasia123"}

	tests := []struct {
		name      string
		mutate    func(r *model.RegisterRequest)
		wantField string
	}{
		{"payload valid", func(r *model.RegisterRequest) {}, ""},
		{"email kosong", func(r *model.RegisterRequest) { r.Email = "" }, "email"},
		{"email tidak valid", func(r *model.RegisterRequest) { r.Email = "bukan-email" }, "email"},
		{"password 73 byte", func(r *model.RegisterRequest) { r.Password = strings.Repeat("a", 73) }, "password"},
		{"password 72 byte diterima", func(r *model.RegisterRequest) { r.Password = strings.Repeat("a", 72) }, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := valid
			tt.mutate(&req)

			err := helper.Validate(req)
			if tt.wantField == "" {
				if err != nil {
					t.Fatalf("tidak boleh error: %v", err)
				}
				return
			}

			var appErr *helper.AppError
			if !errors.As(err, &appErr) || appErr.Fields[tt.wantField] == "" {
				t.Fatalf("harus menghasilkan error pada field %q, dapat: %v", tt.wantField, err)
			}
		})
	}
}

func TestLoginBlockReason(t *testing.T) {
	tests := []struct {
		status string
		want   error
	}{
		{model.AccountActive, nil},
		{model.AccountSuspended, model.ErrAccountSuspended},
		{model.AccountDeleted, model.ErrAccountDeleted},
		{"", model.ErrAccountInactive},
	}

	for _, tt := range tests {
		if got := model.LoginBlockReason(tt.status); !errors.Is(got, tt.want) && got != tt.want {
			t.Fatalf("status %q: dapat %v, ingin %v", tt.status, got, tt.want)
		}
	}
}

func TestRefreshTokenIsUsable(t *testing.T) {
	now := time.Now()
	used := now.Add(-time.Minute)

	if !(model.RefreshToken{ExpiresAt: now.Add(time.Hour)}).IsUsable(now) {
		t.Fatal("token segar harus bisa dipakai")
	}
	if (model.RefreshToken{ExpiresAt: now.Add(time.Hour), UsedAt: &used}).IsUsable(now) {
		t.Fatal("token yang sudah dirotasi tidak boleh dipakai")
	}
	if (model.RefreshToken{ExpiresAt: now.Add(-time.Hour)}).IsUsable(now) {
		t.Fatal("token kedaluwarsa tidak boleh dipakai")
	}
}
