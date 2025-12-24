---
title: File System Operations with ard/fs
description: Read, write, and manage files and directories using Ard's filesystem module.
---

The `ard/fs` module provides functions for working with files and directories in a safe, error-aware manner.

The filesystem module provides:
- **File operations** for reading, writing, and deleting files
- **File inspection** to check if paths are files or directories
- **Directory listing** to discover directory contents
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
