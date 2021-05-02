package main

import (
	"testing"
)

func Test_hasAll(t *testing.T) {
	type args struct {
		has       []string
		requested []string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			"empty arrays",
			args{
				has:       nil,
				requested: nil,
			},
			true,
		},
		{
			"has many requested none",
			args{
				has:       []string{"tag0", "tag1"},
				requested: nil,
			},
			true,
		},
		{
			"has none requested one",
			args{
				has:       nil,
				requested: []string{"tag0"},
			},
			false,
		},
		{
			"has many, including requested one",
			args{
				has:       []string{"tag0", "tag1"},
				requested: []string{"tag0"},
			},
			true,
		},
		{
			"has many, including requested many",
			args{
				has:       []string{"tag0", "tag1", "tag2"},
				requested: []string{"tag0", "tag1"},
			},
			true,
		},
		{
			"has many, not including requested many",
			args{
				has:       []string{"tag0", "tag1", "tag2"},
				requested: []string{"tag0", "tag3"},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasAll(tt.args.has, tt.args.requested); got != tt.want {
				t.Log("has", tt.args.has, "requested", tt.args.requested)
				t.Errorf("hasAll() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getID(t *testing.T) {
	type args struct {
		tags []string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			"just id tag",
			args{[]string{"id:0"}},
			"id:0",
		},
		{
			"multiple tags",
			args{[]string{"tag0", "tag1", "id:0"}},
			"id:0",
		},
		{
			"no id tag",
			args{[]string{"tag0", "tag1", "tag2"}},
			"no id",
		},
		{
			"nil tags",
			args{nil},
			"no id",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getID(tt.args.tags); got != tt.want {
				t.Errorf("getID() = %v, want %v", got, tt.want)
			}
		})
	}
}
