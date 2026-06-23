package gotarget

import "testing"

func TestGoPackageNameFromModulePathSanitizesInvalidNames(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: "user.ard", want: "user"},
		{path: "accounts/foo_bar.ard", want: "foo_bar"},
		{path: "foo-bar.ard", want: "foo_bar"},
		{path: "123-api.ard", want: "_123_api"},
		{path: "type.ard", want: "type_"},
		{path: "---.ard", want: "module"},
	}
	for _, tt := range tests {
		if got := goPackageNameFromModulePath(tt.path); got != tt.want {
			t.Fatalf("goPackageNameFromModulePath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestNaturalGoIdentifierUsesVisibility(t *testing.T) {
	tests := []struct {
		raw      string
		exported bool
		want     string
	}{
		{raw: "make_user", exported: true, want: "MakeUser"},
		{raw: "format_name", exported: false, want: "formatName"},
		{raw: "User", exported: true, want: "User"},
		{raw: "InternalConfig", exported: false, want: "internalConfig"},
		{raw: "type", exported: false, want: "type_"},
	}
	for _, tt := range tests {
		if got := naturalGoIdentifier(tt.raw, tt.exported); got != tt.want {
			t.Fatalf("naturalGoIdentifier(%q, %v) = %q, want %q", tt.raw, tt.exported, got, tt.want)
		}
	}
}
