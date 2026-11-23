package main

import (
	"strings"
	"testing"
)

func TestParseOrder(t *testing.T) {
	cases := []struct {
		name    string
		line    string
		want    Order
		wantErr string
	}{
		{name: "正常", line: "a-1, 100", want: Order{ID: "a-1", Amount: 100}},
		{name: "无逗号", line: "broken", wantErr: "want id,amount"},
		{name: "金额非法", line: "a-1, x", wantErr: `parse amount " x"`},
		{name: "空 id", line: ", 10", want: Order{ID: "", Amount: 10}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseOrder(tc.line)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want contains %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	if err := validate(Order{ID: "a", Amount: 1}); err != nil {
		t.Fatalf("valid order rejected: %v", err)
	}
	err := validate(Order{})
	if err == nil {
		t.Fatal("want error for empty order")
	}
	for _, want := range []string{"empty id", "must be > 0"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %v, want contains %q", err, want)
		}
	}
}
