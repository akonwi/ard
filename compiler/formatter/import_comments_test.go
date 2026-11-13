package formatter

import "testing"

// Comments around imports stay with them when imports are sorted: a comment
// above the first import stays at the top of the file, a comment on a `use`
// line stays on it, and a comment between imports moves with the import
// below it (#583).
func TestFormatKeepsCommentsWithImports(t *testing.T) {
	assertTrailingCommentCases(t, []trailingCommentCase{
		{
			name: "file header stays above the imports",
			input: `// file header

use ard/io

fn main() {
  io::print("x")
}
`,
		},
		{
			name: "file header directly above the imports",
			input: `// file header
use ard/io

fn main() {
  io::print("x")
}
`,
		},
		{
			name: "comments move with their imports when sorted",
			input: `// file header

use ard/io // printing
// the file system
use ard/fs

// doc for main
fn main() {
  io::print(fs::exists("x"))
}
`,
			want: `// file header

// the file system
use ard/fs
use ard/io // printing

// doc for main
fn main() {
  io::print(fs::exists("x"))
}
`,
		},
		{
			name: "comments move with imports across groups",
			input: `use go:example.com/ui // widgets
// standard library
use ard/io

fn main() {
  io::print(ui::Name())
}
`,
			want: `// standard library
use ard/io

use go:example.com/ui // widgets

fn main() {
  io::print(ui::Name())
}
`,
		},
		{
			name: "comments of a removed import are kept in its place",
			input: `use ard/fs // files
// printing
use ard/io

fn main() {
  io::print("x")
}
`,
			want: `// files
// printing
use ard/io

fn main() {
  io::print("x")
}
`,
		},
		{
			name: "file header when every import is removed",
			input: `// file header
use ard/io

fn main() {}
`,
			want: `// file header

fn main() {}
`,
		},
	})
}
