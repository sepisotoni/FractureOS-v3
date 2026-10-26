#!/usr/bin/env python3
"""Bounded root Telnet commands and temporary single-file USB transfers.

Host-side Python only. No telnetlib, expect, FTP, SSH, nc or printf required
on the device. This is a trusted-USB development tool, not the v3 protocol.
"""
import argparse
import hashlib
import http.server
import re
import secrets
import shlex
import socket
import threading
import time
from pathlib import Path


class TelnetStream:
    def __init__(self):
        self.state = "data"
        self.verb = 0

    def feed(self, data):
        out, reply = bytearray(), bytearray()
        for b in data:
            if self.state == "data":
                if b == 255:
                    self.state = "iac"
                else:
                    out.append(b)
            elif self.state == "iac":
                if b == 255:
                    out.append(b)
                    self.state = "data"
                elif b in (251, 252, 253, 254):
                    self.verb, self.state = b, "option"
                elif b == 250:
                    self.state = "sub"
                else:
                    self.state = "data"
            elif self.state == "option":
                if self.verb == 251:
                    reply.extend((255, 253 if b in (1, 3) else 254, b))
                elif self.verb == 253:
                    reply.extend((255, 252, b))
                self.state = "data"
            elif self.state == "sub":
                if b == 255:
                    self.state = "sub_iac"
            elif self.state == "sub_iac":
                self.state = "data" if b == 240 else "sub"
        return bytes(out), bytes(reply)


class TomTom:
    def __init__(self, host="192.168.101.115", timeout=10):
        self.host, self.timeout = host, timeout
        self.sock = socket.create_connection((host, 23), timeout)
        self.parser = TelnetStream()
        self.buffer = bytearray()
        try:
            self.until(rb"login: *$", timeout)
            self.send("root\r\n")
            self.until(rb"(?:^|\n)# *$", timeout)
            self.command("stty -echo; PS1=''; PS2=''", timeout)
        except BaseException:
            self.sock.close()
            raise

    def send(self, text):
        self.sock.sendall(text.encode().replace(b"\xff", b"\xff\xff"))

    def until(self, pattern, timeout, limit=20 * 1024 * 1024):
        end = time.monotonic() + timeout
        while True:
            # NVT CR-NUL and shell CR-CR-LF are not payload line breaks.
            data = bytes(self.buffer).replace(b"\r\x00", b"\r").replace(b"\r", b"")
            match = re.search(pattern, data)
            if match:
                self.buffer.clear()
                return data, match
            if len(self.buffer) > limit:
                raise RuntimeError("Telnet output exceeded limit")
            left = end - time.monotonic()
            if left <= 0:
                raise TimeoutError("Timed out waiting for command completion; remote command may still run")
            self.sock.settimeout(left)
            incoming = self.sock.recv(65536)
            if not incoming:
                raise ConnectionError("TomTom disconnected before command completed")
            plain, response = self.parser.feed(incoming)
            if response:
                self.sock.sendall(response)
            self.buffer.extend(plain)

    def command(self, command, timeout=None):
        token = "__FRACTURE_" + secrets.token_hex(12)
        self.send("{\n" + command + "\n}; fracture_result=$?; echo; echo " + token + "=$fracture_result\r\n")
        data, match = self.until(rb"\n" + token.encode() + rb"=(\d+)\n", timeout or self.timeout)
        return int(match.group(1)), data[:match.start()].decode("utf-8", "replace")

    def checked(self, command, timeout=None):
        code, output = self.command(command, timeout)
        if code:
            raise RuntimeError("Remote exit %d: %s\n%s" % (code, command, output[-1500:]))
        return output

    def upload(self, source, destination, bind="192.168.101.114"):
        payload = Path(source).read_bytes()
        if len(payload) > 8 * 1024 * 1024:
            raise ValueError("Transfer exceeds 8 MiB staging limit")
        route, allowed_host = "/" + secrets.token_hex(20), self.host

        class Handler(http.server.BaseHTTPRequestHandler):
            def do_GET(self):
                if self.path != route or self.client_address[0] != allowed_host:
                    self.send_error(404)
                    return
                self.send_response(200)
                self.send_header("Content-Length", str(len(payload)))
                self.end_headers()
                self.wfile.write(payload)

            def log_message(self, *_args):
                pass

        server = http.server.HTTPServer((bind, 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            url = "http://%s:%d%s" % (bind, server.server_port, route)
            # Never clobber a preexisting remote path.
            self.checked("test ! -e %s && wget -O %s %s" % (
                shlex.quote(destination), shlex.quote(destination), shlex.quote(url)), 30)
        finally:
            server.shutdown()
            server.server_close()
            thread.join()

    def download(self, remote, local, probe):
        size = int(self.checked("wc -c < " + shlex.quote(remote)).strip())
        if not 0 <= size <= 8 * 1024 * 1024:
            raise ValueError("Download exceeds 8 MiB limit")
        data = bytearray()
        # Small independently bounded reads avoid a long-lived process and
        # quadratic buffering over the slow old pseudo-terminal.
        for offset in range(0, max(1, size), 32768):
            count = min(32768, size - offset)
            output = self.checked("%s --hex %s %d %d" % (
                shlex.quote(probe), shlex.quote(remote), offset, count), 15)
            chunk = bytes.fromhex(output)
            if len(chunk) != count:
                raise RuntimeError("Short or changing file during download")
            data.extend(chunk)
        with Path(local).open("xb") as f:
            f.write(data)
        return hashlib.sha256(data).hexdigest()

    def close(self):
        try:
            self.send("exit\r\n")
        except OSError:
            pass
        self.sock.close()

    def __enter__(self):
        return self

    def __exit__(self, *_args):
        self.close()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", default="192.168.101.115")
    sub = parser.add_subparsers(dest="verb", required=True)
    run = sub.add_parser("exec")
    run.add_argument("command")
    run.add_argument("--timeout", type=float, default=10)
    put = sub.add_parser("put")
    put.add_argument("source")
    put.add_argument("destination")
    put.add_argument("--bind", default="192.168.101.114")
    get = sub.add_parser("get")
    get.add_argument("remote")
    get.add_argument("local")
    get.add_argument("--probe", required=True)
    args = parser.parse_args()
    with TomTom(args.host) as device:
        if args.verb == "exec":
            code, output = device.command(args.command, args.timeout)
            print(output, end="")
            return code
        if args.verb == "put":
            device.upload(args.source, args.destination, args.bind)
        if args.verb == "get":
            print(device.download(args.remote, args.local, args.probe))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
