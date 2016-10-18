package main

import (
	"flag"
	"reflect"
	"testing"
)

func TestPlatformFlagPlatforms(t *testing.T) {
	cases := []struct {
		OS        []string
		Arch      []string
		OSArch    []Platform
		Supported []Platform
		Result    []Platform
	}{
		// Building a new list of platforms
		{
			[]string{"foo", "bar"},
			[]string{"baz"},
			[]Platform{},
			[]Platform{
				{"foo", "baz", true},
				{"bar", "baz", true},
				{"boo", "bop", true},
			},
			[]Platform{
				{"foo", "baz", false},
				{"bar", "baz", false},
			},
		},

		// Skipping platforms
		{
			[]string{"!foo"},
			[]string{},
			[]Platform{},
			[]Platform{
				{"foo", "bar", true},
				{"foo", "baz", true},
				{"bar", "bar", true},
			},
			[]Platform{
				{"bar", "bar", false},
			},
		},

		// Specifying only an OS
		{
			[]string{"foo"},
			[]string{},
			[]Platform{},
			[]Platform{
				{"foo", "bar", true},
				{"foo", "baz", true},
				{"bar", "bar", true},
			},
			[]Platform{
				{"foo", "bar", false},
				{"foo", "baz", false},
			},
		},

		// Building a new list, but with some skips
		{
			[]string{"foo", "bar", "!foo"},
			[]string{"baz"},
			[]Platform{},
			[]Platform{
				{"foo", "bar", true},
				{"foo", "baz", true},
				{"bar", "baz", true},
				{"baz", "bar", true},
			},
			[]Platform{
				{"bar", "baz", false},
			},
		},

		// Unsupported pairs
		{
			[]string{"foo", "bar"},
			[]string{"baz"},
			[]Platform{},
			[]Platform{
				{"foo", "baz", true},
				{"bar", "what", true},
			},
			[]Platform{
				{"foo", "baz", false},
			},
		},

		// OSArch basic
		{
			[]string{},
			[]string{},
			[]Platform{
				{"foo", "baz", true},
				{"foo", "bar", true},
			},
			[]Platform{
				{"foo", "baz", true},
				{"bar", "what", true},
			},
			[]Platform{
				{"foo", "baz", false},
			},
		},

		// Negative OSArch
		{
			[]string{},
			[]string{},
			[]Platform{
				{"!foo", "baz", true},
			},
			[]Platform{
				{"foo", "baz", true},
				{"bar", "what", true},
			},
			[]Platform{
				{"bar", "what", false},
			},
		},

		// Mix it all
		{
			[]string{"foo", "bar"},
			[]string{"bar"},
			[]Platform{
				{"foo", "baz", true},
				{"!bar", "bar", true},
			},
			[]Platform{
				{"foo", "bar", true},
				{"foo", "baz", true},
				{"bar", "bar", true},
			},
			[]Platform{
				{"foo", "baz", false},
				{"foo", "bar", false},
			},
		},

		// Ignores non-default
		{
			[]string{},
			[]string{},
			[]Platform{},
			[]Platform{
				{"foo", "bar", true},
				{"foo", "baz", true},
				{"bar", "bar", false},
			},
			[]Platform{
				{"foo", "bar", false},
				{"foo", "baz", false},
			},
		},

		// Adds non-default by OS
		{
			[]string{"bar"},
			[]string{},
			[]Platform{},
			[]Platform{
				{"foo", "bar", true},
				{"foo", "baz", true},
				{"bar", "bar", false},
			},
			[]Platform{
				{"bar", "bar", false},
			},
		},

		// Adds non-default by both
		{
			[]string{"bar"},
			[]string{"bar"},
			[]Platform{},
			[]Platform{
				{"foo", "bar", true},
				{"foo", "baz", true},
				{"bar", "bar", false},
			},
			[]Platform{
				{"bar", "bar", false},
			},
		},
	}

	for _, tc := range cases {
		f := PlatformFlag{
			OS:     tc.OS,
			Arch:   tc.Arch,
			OSArch: tc.OSArch,
		}

		result := f.Platforms(tc.Supported)
		if !reflect.DeepEqual(result, tc.Result) {
			t.Errorf("input: %#v\nresult: %#v", f, result)
		}
	}
}

func TestPlatformFlagArchFlagValue(t *testing.T) {
	var f PlatformFlag
	val := f.ArchFlagValue()
	if err := val.Set("foo bar"); err != nil {
		t.Fatalf("err: %s", err)
	}

	expected := []string{"foo", "bar"}
	if !reflect.DeepEqual(f.Arch, expected) {
		t.Fatalf("bad: %#v", f.Arch)
	}
}

func TestPlatformFlagOSArchFlagValue(t *testing.T) {
	var f PlatformFlag
	val := f.OSArchFlagValue()
	if err := val.Set("foo/bar"); err != nil {
		t.Fatalf("err: %s", err)
	}

	expected := []Platform{{"foo", "bar", false}}
	if !reflect.DeepEqual(f.OSArch, expected) {
		t.Fatalf("bad: %#v", f.OSArch)
	}
}

func TestPlatformFlagOSFlagValue(t *testing.T) {
	var f PlatformFlag
	val := f.OSFlagValue()
	if err := val.Set("foo bar"); err != nil {
		t.Fatalf("err: %s", err)
	}

	expected := []string{"foo", "bar"}
	if !reflect.DeepEqual(f.OS, expected) {
		t.Fatalf("bad: %#v", f.OS)
	}
}

func TestAppendPlatformValue_impl(t *testing.T) {
	var _ flag.Value = new(appendPlatformValue)
}

func TestAppendPlatformValue(t *testing.T) {
	var value appendPlatformValue

	if err := value.Set(""); err != nil {
		t.Fatalf("err: %s", err)
	}

	if len(value) > 0 {
		t.Fatalf("bad: %#v", value)
	}

	if err := value.Set("windows/arm/bad"); err == nil {
		t.Fatal("should err")
	}

	if err := value.Set("windows"); err == nil {
		t.Fatal("should err")
	}

	if err := value.Set("windows/arm windows/386"); err != nil {
		t.Fatalf("err: %s", err)
	}

	expected := []Platform{
		{"windows", "arm", false},
		{"windows", "386", false},
	}
	if !reflect.DeepEqual([]Platform(value), expected) {
		t.Fatalf("bad: %#v", value)
	}
}

func TestAppendStringValue_impl(t *testing.T) {
	var _ flag.Value = new(appendStringValue)
}

func TestAppendStringValue(t *testing.T) {
	var value appendStringValue

	if err := value.Set(""); err != nil {
		t.Fatalf("err: %s", err)
	}

	if len(value) > 0 {
		t.Fatalf("bad: %#v", value)
	}

	if err := value.Set("windows LINUX"); err != nil {
		t.Fatalf("err: %s", err)
	}

	expected := []string{"windows", "linux"}
	if !reflect.DeepEqual([]string(value), expected) {
		t.Fatalf("bad: %#v", value)
	}

	if err := value.Set("darwin"); err != nil {
		t.Fatalf("err: %s", err)
	}

	expected = []string{"windows", "linux", "darwin"}
	if !reflect.DeepEqual([]string(value), expected) {
		t.Fatalf("bad: %#v", value)
	}

	if err := value.Set("darwin"); err != nil {
		t.Fatalf("err: %s", err)
	}

	expected = []string{"windows", "linux", "darwin"}
	if !reflect.DeepEqual([]string(value), expected) {
		t.Fatalf("bad: %#v", value)
	}
}
