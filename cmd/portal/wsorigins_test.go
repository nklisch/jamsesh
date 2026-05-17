package main

import (
	"reflect"
	"testing"
)

func TestParseAllowOrigins(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "empty string returns nil (deny all cross-origin)",
			in:   "",
			want: nil,
		},
		{
			name: "single origin",
			in:   "https://app.example.com",
			want: []string{"https://app.example.com"},
		},
		{
			name: "multiple origins comma-separated",
			in:   "https://app.example.com,http://localhost:5173",
			want: []string{"https://app.example.com", "http://localhost:5173"},
		},
		{
			name: "whitespace around commas is trimmed",
			in:   "  https://app.example.com  ,  http://localhost:5173  ",
			want: []string{"https://app.example.com", "http://localhost:5173"},
		},
		{
			name: "empty entries dropped",
			in:   ",,https://a.test,,https://b.test,",
			want: []string{"https://a.test", "https://b.test"},
		},
		{
			name: "whitespace-only entries dropped",
			in:   "  ,  https://a.test ,   ,https://b.test, \t",
			want: []string{"https://a.test", "https://b.test"},
		},
		{
			name: "single comma returns nil",
			in:   ",",
			want: nil,
		},
		{
			name: "only whitespace returns nil",
			in:   "   \t  ",
			want: nil,
		},
		{
			name: "trailing comma after single entry",
			in:   "https://a.test,",
			want: []string{"https://a.test"},
		},
		{
			name: "leading comma before single entry",
			in:   ",https://a.test",
			want: []string{"https://a.test"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseAllowOrigins(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseAllowOrigins(%q) = %#v; want %#v", tc.in, got, tc.want)
			}
		})
	}
}
