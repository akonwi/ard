import fs from "node:fs";

let stdinBuffer = "";
let stdinEOF = false;

function fillStdinBuffer() {
  const chunk = Buffer.alloc(4096);
  const bytesRead = fs.readSync(0, chunk, 0, chunk.length, null);
  if (bytesRead === 0) {
    stdinEOF = true;
    return;
  }
  stdinBuffer += chunk.toString("utf8", 0, bytesRead);
}

export function printLine(value) {
  process.stdout.write(String(value) + "\n");
}

export function readLine() {
  try {
    while (true) {
      const newlineIndex = stdinBuffer.indexOf("\n");
      if (newlineIndex >= 0) {
        let line = stdinBuffer.slice(0, newlineIndex);
        stdinBuffer = stdinBuffer.slice(newlineIndex + 1);
        if (line.endsWith("\r")) {
          line = line.slice(0, -1);
        }
        return { ok: line };
      }

      if (stdinEOF) {
        let line = stdinBuffer;
        stdinBuffer = "";
        if (line.endsWith("\r")) {
          line = line.slice(0, -1);
        }
        return { ok: line };
      }

      fillStdinBuffer();
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { err: message };
  }
}
