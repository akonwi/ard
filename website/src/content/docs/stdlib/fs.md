---
title: File System Operations with ard/fs
description: Read, write, and manage files and directories using Ard's filesystem module.
---

The `ard/fs` module provides functions for working with files and directories in a safe, error-aware manner.

The filesystem module provides:
- **File operations** for reading, writing, copying, and deleting files
- **File inspection** to check existence, type, and size of paths
- **Directory operations** for creating, listing, and deleting directories
- **Path utilities** for resolving the current working directory and absolute paths
- **Result types** for proper error handling

```ard
use ard/fs
use ard/io

fn main() {
  // Check if a file exists
  if fs::is_file("data.txt") {
    let content = fs::read("data.txt").expect("Failed to read")
    io::print(content)
  }
  
  // List directory contents
  match fs::list_dir(".") {
    ok(entries) => {
      for entry in entries {
        io::print(entry.name)
      }
    },
    err(e) => io::print("Error: {e}")
  }
}
```

## API

### `fn exists(path: Str) Bool`

Check if something exists at the given path, whether it's a file or directory.

```ard
use ard/fs

if fs::exists("config.json") {
  // file or directory exists
}
```

### `fn is_file(path: Str) Bool`

Check if the path points to a regular file. Returns false for directories or non-existent paths.

```ard
use ard/fs

if fs::is_file("document.txt") {
  // it's a file
}
```

### `fn is_dir(path: Str) Bool`

Check if the path points to a directory. Returns false for files or non-existent paths.

```ard
use ard/fs

if fs::is_dir("./src") {
  // it's a directory
}
```

### `fn read(path: Str) Str!Str`

Read the entire contents of a file and return it as a string, or an error if the operation fails.

```ard
use ard/fs

let content = fs::read("file.txt").expect("Failed to read file")
```

### `fn write(path: Str, content: Str) Void!Str`

Write content to a file. Creates the file if it doesn't exist, or overwrites it if it does.

```ard
use ard/fs

fs::write("output.txt", "Hello, World!").expect("Failed to write")
```

### `fn append(path: Str, content: Str) Void!Str`

Append content to the end of a file. Creates the file if it doesn't exist.

```ard
use ard/fs

fs::append("log.txt", "New log entry\n").expect("Failed to append")
```

### `fn create_file(path: Str) Bool!Str`

Create a new empty file. Returns an error if the file already exists.

```ard
use ard/fs

fs::create_file("newfile.txt").expect("Failed to create file")
```

### `fn delete(path: Str) Void!Str`

Delete a file. Returns an error if the file doesn't exist.

```ard
use ard/fs

fs::delete("tempfile.txt").expect("Failed to delete")
```

### `fn copy(from: Str, to: Str) Void!Str`

Copy a file from one path to another. Creates the destination file if it doesn't exist, or overwrites it if it does.

```ard
use ard/fs

fs::copy("original.txt", "backup.txt").expect("Failed to copy")
```

### `fn rename(from: Str, to: Str) Void!Str`

Move or rename a file or directory. The source is removed after a successful rename.

```ard
use ard/fs

fs::rename("old_name.txt", "new_name.txt").expect("Failed to rename")
```

### `fn file_size(path: Str) Int!Str`

Get the size of a file in bytes.

```ard
use ard/fs

let size = fs::file_size("data.bin").expect("Failed to get size")
```

## Path Utilities

### `fn cwd() Str!Str`

Get the current working directory.

```ard
use ard/fs

let dir = fs::cwd().expect("Failed to get cwd")
```

### `fn abs(path: Str) Str!Str`

Resolve a relative path to an absolute path.

```ard
use ard/fs

let full_path = fs::abs("./src").expect("Failed to resolve path")
```

## Directory Operations

### `fn create_dir(path: Str) Void!Str`

Create a directory at the given path, including any missing parent directories. Returns an error if the operation fails.

```ard
use ard/fs

fs::create_dir("output/reports/2024").expect("Failed to create directory")
```

### `fn delete_dir(path: Str) Void!Str`

Delete a directory and all its contents recursively. Returns an error if the operation fails.

```ard
use ard/fs

fs::delete_dir("build/output").expect("Failed to delete directory")
```

## Directory Listing

### `struct DirEntry`

Represents a single entry in a directory.

- **`name: Str`** - The name of the file or directory
- **`is_file: Bool`** - True if it's a file, false if it's a directory

### `fn list_dir(path: Str) [DirEntry]!Str`

List all entries in a directory. Returns a result containing a list of `DirEntry` structs or an error.

```ard
use ard/fs
use ard/io

let db = fs::list_dir("./data").expect("Failed to list directory")

for entry in entries {
  if entry.is_file {
    io::print("File: {entry.name}")
  } else {
    io::print("Dir: {entry.name}")
  }
}
```
