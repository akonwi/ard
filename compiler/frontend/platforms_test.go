package frontend

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/akonwi/ard/compiler/platform"
)

func writeProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files["ard.toml"] = "name = \"app\"\nard = \">= 0.1.0\"\n"
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, "main.ard")
}

func TestRepresentativePlatforms(t *testing.T) {
	darwin := platform.Platform{OS: "darwin", Arch: "arm64"}
	linux := platform.Platform{OS: "linux", Arch: "amd64"}
	tests := []struct {
		name  string
		files map[string]string
		host  platform.Platform
		want  []platform.Platform
	}{
		{
			name:  "no platform attributes",
			files: map[string]string{"main.ard": "use app/a\n\nfn main() {}\n", "a.ard": "fn f() {}\n"},
			host:  darwin,
			want:  []platform.Platform{darwin},
		},
		{
			name: "unix and the rest",
			files: map[string]string{
				"main.ard": "use app/a\n\nfn main() {}\n",
				"a.ard":    "fn f() {}\n\n#platform(unix)\nfn f() {}\n",
			},
			host: darwin,
			want: []platform.Platform{darwin, {OS: "windows", Arch: "amd64"}},
		},
		{
			name: "module imported on one platform",
			files: map[string]string{
				"main.ard":   "#platform(windows)\nuse app/win\n\nfn main() {}\n",
				"win.ard":    "fn f() {}\n\n#platform(windows_arm64)\nfn f() {}\n",
				"unused.ard": "#platform(plan9)\nfn g() {}\n",
			},
			host: linux,
			want: []platform.Platform{linux, {OS: "windows", Arch: "amd64"}, {OS: "windows", Arch: "arm64"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mainPath := writeProject(t, tt.files)
			got, err := RepresentativePlatforms(mainPath, tt.host)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("RepresentativePlatforms = %v, want %v", got, tt.want)
			}
		})
	}
}
