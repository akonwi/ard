#!/usr/bin/env python3
import fcntl
import os
import pty
import select
import shlex
import signal
import struct
import subprocess
import sys
import termios
import time

ROOT = os.path.dirname(os.path.abspath(__file__))
BIN = os.path.join(ROOT, "ttt")
ARD_CMD = shlex.split(os.environ.get("ARD", "ard"))


class Screen:
    def __init__(self, rows=24, cols=100):
        self.rows = rows
        self.cols = cols
        self.row = 0
        self.col = 0
        self.cells = [[" " for _ in range(cols)] for _ in range(rows)]

    def text(self):
        return "\n".join("".join(row).rstrip() for row in self.cells)

    def feed(self, data):
        text = data.decode("utf-8", errors="ignore").replace("\x00", "")
        i = 0
        while i < len(text):
            ch = text[i]
            if ch == "\x1b":
                i = self._escape(text, i + 1)
                continue
            if ch == "\r":
                self.col = 0
            elif ch == "\n":
                self.row = min(self.rows - 1, self.row + 1)
                self.col = 0
            elif ch == "\b":
                self.col = max(0, self.col - 1)
            elif ch >= " ":
                if 0 <= self.row < self.rows and 0 <= self.col < self.cols:
                    self.cells[self.row][self.col] = ch
                self.col += 1
                if self.col >= self.cols:
                    self.col = 0
                    self.row = min(self.rows - 1, self.row + 1)
            i += 1

    def _escape(self, text, i):
        if i >= len(text):
            return i
        kind = text[i]
        if kind == "[":
            j = i + 1
            while j < len(text) and not ("@" <= text[j] <= "~"):
                j += 1
            if j >= len(text):
                return j
            body = text[i + 1:j]
            final = text[j]
            self._csi(body, final)
            return j + 1
        if kind in "]P_":
            end = text.find("\x1b\\", i + 1)
            if end == -1:
                bel = text.find("\a", i + 1)
                return len(text) if bel == -1 else bel + 1
            return end + 2
        return i + 1

    def _csi(self, body, final):
        clean = body.lstrip("?")
        parts = [p for p in clean.split(";") if p]
        nums = []
        for part in parts:
            try:
                nums.append(int(part))
            except ValueError:
                nums.append(0)
        if final in "Hf":
            row = nums[0] if len(nums) >= 1 and nums[0] else 1
            col = nums[1] if len(nums) >= 2 and nums[1] else 1
            self.row = max(0, min(self.rows - 1, row - 1))
            self.col = max(0, min(self.cols - 1, col - 1))
        elif final == "J" and (not nums or nums[0] == 2):
            self.cells = [[" " for _ in range(self.cols)] for _ in range(self.rows)]
            self.row = 0
            self.col = 0


def build():
    subprocess.run([*ARD_CMD, "build", "--out", "ttt", "main.ard"], cwd=ROOT, check=True)


def spawn():
    pid, fd = pty.fork()
    if pid == 0:
        os.chdir(ROOT)
        os.environ.setdefault("TERM", "xterm-256color")
        os.execv(BIN, [BIN])
    set_winsize(fd, 24, 100)
    return pid, fd


def set_winsize(fd, rows, cols):
    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))


def read_for(fd, screen=None, seconds=0.15):
    deadline = time.time() + seconds
    chunks = []
    while time.time() < deadline:
        timeout = max(0, deadline - time.time())
        ready, _, _ = select.select([fd], [], [], timeout)
        if not ready:
            continue
        try:
            data = os.read(fd, 65536)
        except OSError:
            break
        if not data:
            break
        respond_to_terminal_queries(fd, data)
        if screen is not None:
            screen.feed(data)
        chunks.append(data.decode("utf-8", errors="ignore").replace("\x00", ""))
    return "".join(chunks)


def respond_to_terminal_queries(fd, data):
    responses = []
    if b"\x1b[6n" in data:
        responses.append(b"\x1b[1;1R")
    if b"\x1b[c" in data:
        responses.append(b"\x1b[?1;0c")
    if b"\x1b[=c" in data:
        responses.append(b"\x1b[>0;0;0c")
    for response in responses:
        os.write(fd, response)


def wait_for_screen(fd, screen, needle, timeout=2.0):
    deadline = time.time() + timeout
    raw = ""
    while time.time() < deadline:
        raw += read_for(fd, screen, 0.05)
        rendered = screen.text()
        if needle in rendered:
            return rendered
    raise AssertionError(f"did not see {needle!r}; screen:\n{screen.text()}\nraw tail:\n{raw[-2000:]}")


def send(fd, text):
    os.write(fd, text.encode())


def main():
    build()
    pid, fd = spawn()
    screen = Screen()
    try:
        wait_for_screen(fd, screen, "Player X to move")
        wait_for_screen(fd, screen, "[ ]")

        send(fd, "\r")
        wait_for_screen(fd, screen, "Player O to move")
        wait_for_screen(fd, screen, "[X]")

        send(fd, "r")
        wait_for_screen(fd, screen, "New game. Choose a square.")
        wait_for_screen(fd, screen, "Player X to move")
        wait_for_screen(fd, screen, "[ ]")

        send(fd, "q")
        deadline = time.time() + 2.0
        status = None
        while time.time() < deadline:
            done, maybe_status = os.waitpid(pid, os.WNOHANG)
            if done == pid:
                status = maybe_status
                break
            read_for(fd, screen, 0.05)
        if status is None:
            send(fd, "\x03")
            raise AssertionError("tic-tac-toe did not exit after q")
        if status != 0:
            raise AssertionError(f"tic-tac-toe exited with status {status}")
        print("tic-tac-toe smoke test passed")
    finally:
        try:
            os.close(fd)
        except OSError:
            pass
        try:
            os.kill(pid, signal.SIGTERM)
        except OSError:
            pass


if __name__ == "__main__":
    try:
        main()
    except Exception as err:
        print(f"FAIL: {err}", file=sys.stderr)
        sys.exit(1)
