package vm_test

import "testing"

func TestFS(t *testing.T) {
	runTests(t, []test{
		{
			name: "fs::exists returns false when there is something at the given path",
			input: `
			  use ard/fs
				fs::exists("path/to/file")`,
			want: false,
		},
		{
			name: "fs::exists returns true when there is something at the given path",
			input: `
					  use ard/fs
						fs::exists("../main.go")`,
			want: true,
		},
		{
			name: "fs::create_file returns Bool!Str",
			input: `
				use ard/fs
				fs::create_file("./fixtures/fake.file").expect("Failed to create file")
				`,
			want: true,
		},
		{
			name: "fs::write returns Void!Str",
			input: `
				use ard/fs
				fs::write("./fixtures/fake.file", "content")`,
			want: nil,
		},
		{
			name: "fs::append returns Void!Str",
			input: `
				use ard/fs
				fs::append("./fixtures/fake.file", "-appended")`,
			want: nil,
		},
		{
			name: "fs::read returns maybe.some with the file contents, when there is a file at the given path",
			input: `
				use ard/fs
				match fs::read("./fixtures/fake.file") {
					ok(s) => s,
					err => err,
				}`,
			want: "content-appended",
		},
		{
			name: "fs::read returns an empty maybe when there is nothing at the given path",
			input: `
				use ard/fs
				fs::read("foo").is_err()
				`,
			want: true,
		},
		{
			name: "fs::delete returns Void!Str",
			input: `
				use ard/fs
				fs::delete("./fixtures/fake.file")`,
			want: nil,
		},
	})
}
