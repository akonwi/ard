package vm_test

import "testing"

func TestVM(t *testing.T) {
	runTests(t, []test{
		{
			name: "fs.exists returns false when there is something at the given path",
			input: `
			  use ard/fs
				fs.exists("path/to/file")`,
			want: false,
		},
		{
			name: "fs.exists returns true when there is something at the given path",
			input: `
					  use ard/fs
						fs.exists("../main.go")`,
			want: true,
		},
		{
			name: "fs.create_file returns a Bool indicating success",
			input: `
				use ard/fs
				fs.create_file("./fixtures/fake.file")
				`,
			want: true,
		},
		{
			name: "fs.write returns a Bool indicating success",
			input: `
				use ard/fs
				fs.write("./fixtures/fake.file", "content")`,
			want: true,
		},
		{
			name: "fs.append returns a Bool indicating success",
			input: `
				use ard/fs
				fs.append("./fixtures/fake.file", "line 2")`,
			want: true,
		},
		{
			name: "fs.read returns maybe.some with the file contents, when there is a file at the given path",
			input: `
				use ard/fs
				match fs.read("./fixtures/fake.file") {
					s => s,
					_ => "no file",
				}`,
			want: "content\nline 2",
		},
		{
			name: "fs.read returns an empty maybe when there is nothing at the given path",
			input: `
				use ard/fs
				match fs.read("foo") {
					s => s,
				  _ => "no file",
				}`,
			want: "no file",
		},
		{
			name: "fs.delete returns a Bool indicating success",
			input: `
				use ard/fs
				fs.delete("./fixtures/fake.file")`,
			want: true,
		},
	})
}
